//go:build !windows

package supervisor

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/lestrrat-go/server-starter/v2/internal/statefile"
	"github.com/stretchr/testify/require"
)

// stubbornWorkerTxt ignores the signal start_server sends on HUP, so it stays
// in the old-workers set instead of exiting. That is what gives a second HUP
// something to re-signal. It still dies on TERM, which is what the run sends
// its workers on shutdown, so the run can be torn down normally.
var stubbornWorkerTxt = `package main

import (
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	signal.Ignore(syscall.SIGUSR1)
	if len(os.Args) == 3 && os.Args[1] == "ignore-term" {
		signal.Ignore(syscall.SIGTERM)
		if err := os.WriteFile(os.Args[2], []byte("ready"), 0600); err != nil {
			panic(err)
		}
		for {
			time.Sleep(time.Hour)
		}
	}

	term := make(chan os.Signal, 1)
	signal.Notify(term, syscall.SIGTERM)
	<-term
}
`

// buildStubbornWorker compiles stubbornWorkerTxt into dir and returns its path.
func buildStubbornWorker(t *testing.T, dir string) string {
	t.Helper()

	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(stubbornWorkerTxt), 0600); err != nil {
		t.Fatalf("failed to write worker source: %s", err)
	}
	// The worker imports nothing outside the standard library, so the scratch
	// module needs no replace directive back to this repository.
	goMod := "module server-starter-stubborn\n\ngo 1.23\n"
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(goMod), 0600); err != nil {
		t.Fatalf("failed to write go.mod: %s", err)
	}

	bin := filepath.Join(dir, "stubborn")
	cmd := exec.CommandContext(context.Background(), "go", "build", "-buildvcs=false", "-o", bin, ".")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to compile worker: %s\n%s", err, output)
	}
	return bin
}

// generations reports the generation numbers currently listed in the status
// file. The file is replaced via temp-file-plus-rename on every update, so a
// read always sees either the previous complete file or the new one, never a
// torn write; callers still poll because the file's *contents* change
// asynchronously as the supervisor processes each signal.
func generations(t *testing.T, statusFile string) []string {
	t.Helper()

	buf, err := os.ReadFile(statusFile)
	if err != nil {
		return nil
	}

	trimmed := strings.TrimSpace(string(buf))
	if trimmed == "" {
		return nil
	}

	var gens []string
	for _, line := range strings.Split(trimmed, "\n") {
		gen, _, _ := strings.Cut(line, ":")
		gens = append(gens, gen)
	}
	return gens
}

// waitForGenerations blocks until the status file lists want generations.
func waitForGenerations(t *testing.T, statusFile string, want int) {
	t.Helper()

	deadline := time.Now().Add(10 * time.Second)
	var last []string
	for time.Now().Before(deadline) {
		last = generations(t, statusFile)
		if len(last) == want {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %d generations in status file, last saw %v", want, last)
}

func waitForGenerationList(t *testing.T, statusFile string, want []string) {
	t.Helper()

	deadline := time.Now().Add(10 * time.Second)
	var last []string
	for time.Now().Before(deadline) {
		last = generations(t, statusFile)
		if strings.Join(last, ",") == strings.Join(want, ",") {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for generations %v in status file, last saw %v", want, last)
}

func waitForDiagnostic(t *testing.T, stderr *syncBuffer, want string) {
	t.Helper()

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(stderr.String(), want) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for diagnostic %q; output:\n%s", want, stderr.String())
}

// TestHUPCoalescesWhileOldWorkerLives verifies that repeated restart requests
// cannot accumulate worker generations. One request stays pending until the
// old worker exits, then causes exactly one additional restart.
func TestHUPCoalescesWhileOldWorkerLives(t *testing.T) {
	dir := t.TempDir()
	statusFile := filepath.Join(dir, "status")
	var stderr syncBuffer

	sd, err := NewStarter(&config{
		command:    buildStubbornWorker(t, dir),
		statusfile: statusFile,
		// The worker ignores USR1, so the first old worker stays live while
		// the second HUP is processed.
		sigonhup: "USR1",
		stderr:   &stderr,
	})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ctrl, err := sd.Run(ctx)
	require.NoError(t, err)
	defer func() {
		cancel()
		select {
		case <-ctrl.Done():
		case <-time.After(10 * time.Second):
			t.Error("timed out waiting for Run() to return")
		}
	}()

	waitForGenerationList(t, statusFile, []string{"1"})
	ctrl.Hangup()
	waitForGenerationList(t, statusFile, []string{"1", "2"})

	ctrl.Hangup()
	waitForDiagnostic(t, &stderr, "coalescing hangup request until old workers exit")
	require.Equal(t, []string{"1", "2"}, generations(t, statusFile))

	status, err := statefile.ReadStatus(statusFile)
	require.NoError(t, err)
	oldWorker, err := os.FindProcess(status[1])
	require.NoError(t, err)
	require.NoError(t, oldWorker.Signal(syscall.SIGTERM))

	waitForGenerationList(t, statusFile, []string{"2", "3"})
}

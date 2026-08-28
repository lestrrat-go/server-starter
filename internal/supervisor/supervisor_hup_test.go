//go:build !windows

package supervisor

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
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
)

func main() {
	signal.Ignore(syscall.SIGUSR1)

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

// TestHUPWithLiveOldWorkers covers #9. Server::Starter respawns and re-signals
// on every HUP; this port used to gate that on the old-worker set being empty,
// so a HUP arriving while a previous worker was still alive did nothing at all.
func TestHUPWithLiveOldWorkers(t *testing.T) {
	dir := t.TempDir()
	statusFile := filepath.Join(dir, "status")

	sd, err := NewStarter(&config{
		command:    buildStubbornWorker(t, dir),
		statusfile: statusFile,
		// The worker ignores USR1, so old workers survive each HUP and pile up.
		sigonhup: "USR1",
	})
	if err != nil {
		t.Fatalf("failed to create starter: %s", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ctrl, err := sd.Run(ctx)
	if err != nil {
		t.Fatalf("sd.Run() failed: %s", err)
	}
	defer func() {
		cancel()
		select {
		case <-ctrl.Done():
		case <-time.After(10 * time.Second):
			t.Errorf("timed out waiting for Run() to return")
		}
	}()

	// Generation 0, no old workers yet.
	waitForGenerations(t, statusFile, 1)

	// First hangup: this worked even before the fix, because the old-worker
	// set was still empty at this point.
	ctrl.Hangup()
	waitForGenerations(t, statusFile, 2)

	// Second hangup, now with generation 0 still alive and ignoring its
	// signal. Before the fix the starter treated this as a no-op and the
	// status file never grew a third entry.
	ctrl.Hangup()
	waitForGenerations(t, statusFile, 3)

	// Generations are 1-based: startWorker increments before spawning. All
	// three must still be listed, which is what proves the older workers were
	// kept and re-signalled rather than dropped.
	gens := make(map[string]struct{})
	for _, gen := range generations(t, statusFile) {
		gens[gen] = struct{}{}
	}
	for want := 1; want <= 3; want++ {
		if _, ok := gens[fmt.Sprintf("%d", want)]; !ok {
			t.Errorf("generation %d missing from status file, got %v", want, gens)
		}
	}
}

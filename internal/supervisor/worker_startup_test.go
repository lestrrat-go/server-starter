//go:build !windows

package supervisor

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

const failingReplacementWorkerTxt = `package main

import (
	"os"
	"os/signal"
	"strconv"
	"syscall"
)

func main() {
	marker := os.Args[1]
	generation, _ := strconv.Atoi(os.Getenv("SERVER_STARTER_GENERATION"))
	if generation != 1 {
		_ = os.WriteFile(marker, []byte("started"), 0600)
		os.Exit(1)
	}

	term := make(chan os.Signal, 1)
	signal.Notify(term, syscall.SIGTERM)
	<-term
}
`

func buildFailingReplacementWorker(t *testing.T, dir string) string {
	t.Helper()

	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(failingReplacementWorkerTxt), 0600); err != nil {
		t.Fatalf("failed to write worker source: %s", err)
	}
	goMod := "module server-starter-failing-replacement\n\ngo 1.23\n"
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(goMod), 0600); err != nil {
		t.Fatalf("failed to write go.mod: %s", err)
	}

	bin := filepath.Join(dir, "failing-replacement")
	cmd := exec.CommandContext(context.Background(), "go", "build", "-buildvcs=false", "-o", bin, ".")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to compile worker: %s\n%s", err, output)
	}
	return bin
}

func waitForFile(t *testing.T, path string) {
	t.Helper()

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", path)
}

func TestSIGTERMDuringFailedReplacementDoesNotPanic(t *testing.T) {
	dir := t.TempDir()
	statusFile := filepath.Join(dir, "status")
	marker := filepath.Join(dir, "replacement-started")

	sd, err := NewStarter(&config{
		args:       []string{marker},
		command:    buildFailingReplacementWorker(t, dir),
		interval:   1,
		statusfile: statusFile,
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

	waitForGenerations(t, statusFile, 1)
	ctrl.Hangup()
	waitForFile(t, marker)
	cancel()

	select {
	case <-ctrl.Done():
	case <-time.After(10 * time.Second):
		t.Fatalf("timed out waiting for Run() to return")
	}
	if err := ctrl.Err(); err != nil && !errors.Is(err, ErrServerClosed) {
		t.Errorf("sd.Run() failed: %s", err)
	}
}

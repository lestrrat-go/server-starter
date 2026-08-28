package supervisor

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestRunConcurrentOnSharedStarter proves that Run is safe to invoke as
// `go sd.Run(ctx)`: the same *Starter, sharing one command and one port spec
// (port 0, so each run binds its own ephemeral port instead of colliding),
// is run twice concurrently, each with its own context. Both runs must
// complete cleanly when their own context is cancelled, independently of
// each other, and each must have spawned its own live worker process.
//
// statusFile and pidFile are left empty on purpose: WriteStatus is a no-op
// for an empty path, and the temp filename statefile.Acquire would derive
// from the process id is identical for both runs, so a shared pidFile would
// collide regardless of the fix under test here.
func TestRunConcurrentOnSharedStarter(t *testing.T) {
	dir := t.TempDir()
	markerFile := filepath.Join(dir, "workers.txt")

	// The worker command appends its own pid to markerFile, then sleeps.
	// Using a shell command keeps this file buildable on Windows (it only
	// needs to compile there) without needing a companion worker binary.
	// "exec sleep" replaces the shell with sleep in place (same pid), so
	// the TERM the supervisor sends terminates it directly instead of
	// leaving an orphaned grandchild holding the test's stdout pipe open.
	sd, err := NewStarter(&config{
		command: "/bin/sh",
		args:    []string{"-c", `echo $$ >> "$1"; exec sleep 30`, "sh", markerFile},
		ports:   []string{"0"},
	})
	require.NoError(t, err)

	const numRuns = 2
	ctrls := make([]*Controller, numRuns)
	cancels := make([]context.CancelFunc, numRuns)
	for i := range numRuns {
		ctx, cancel := context.WithCancel(context.Background())
		cancels[i] = cancel
		defer cancel()

		ctrl, err := sd.Run(ctx)
		require.NoError(t, err)
		ctrls[i] = ctrl
	}

	pids := waitForWorkerPids(t, markerFile, numRuns, 20*time.Second)
	require.GreaterOrEqual(t, len(pids), numRuns, "expected at least %d distinct worker pids in %s, got %v", numRuns, markerFile, pids)

	// Cancel each run's own context independently: this is what proves the
	// two runs are truly independent, rather than sharing one process-wide
	// shutdown signal as before.
	errs := make([]error, numRuns)
	doneCh := make(chan struct{}, numRuns)
	for i := range numRuns {
		go func(i int) {
			defer func() { doneCh <- struct{}{} }()
			cancels[i]()
			errs[i] = ctrls[i].Wait()
		}(i)
	}

	for i := range numRuns {
		select {
		case <-doneCh:
		case <-time.After(20 * time.Second):
			t.Fatalf("timed out waiting for Run() #%d to return", i)
		}
	}
	for i := range numRuns {
		require.ErrorIs(t, errs[i], ErrServerClosed)
	}
}

// waitForWorkerPids polls markerFile until it contains at least want
// distinct lines (worker pids), or the timeout elapses. It returns the
// distinct lines seen so the caller can report them on failure.
func waitForWorkerPids(t *testing.T, markerFile string, want int, timeout time.Duration) map[string]struct{} {
	t.Helper()

	pids := make(map[string]struct{})
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(markerFile)
		if err == nil {
			pids = make(map[string]struct{})
			for _, line := range strings.Fields(string(data)) {
				pids[line] = struct{}{}
			}
			if len(pids) >= want {
				return pids
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	return pids
}

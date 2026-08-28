package supervisor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestRunConcurrentOnSharedStarter proves that Run is safe to invoke as
// `go sd.Run()`: the same *Starter, sharing one command and one port spec
// (port 0, so each run binds its own ephemeral port instead of colliding),
// is run twice concurrently. Both runs must complete without error, and
// each must have spawned its own live worker process.
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
	// the SIGTERM the supervisor sends terminates it directly instead of
	// leaving an orphaned grandchild holding the test's stdout pipe open.
	sd, err := NewStarter(&config{
		command: "/bin/sh",
		args:    []string{"-c", `echo $$ >> "$1"; exec sleep 30`, "sh", markerFile},
		ports:   []string{"0"},
	})
	require.NoError(t, err)

	const numRuns = 2
	errCh := make(chan error, numRuns)
	doneCh := make(chan struct{}, numRuns)
	for range numRuns {
		go func() {
			defer func() { doneCh <- struct{}{} }()
			errCh <- sd.Run()
		}()
	}

	pids := waitForWorkerPids(t, markerFile, numRuns, 20*time.Second)
	require.GreaterOrEqual(t, len(pids), numRuns, "expected at least %d distinct worker pids in %s, got %v", numRuns, markerFile, pids)

	// A single SIGTERM to this process is delivered to both runs: each Run
	// registers its own channel with signal.Notify, so one call to stop()
	// shuts both down.
	sd.stop()

	for i := range numRuns {
		select {
		case <-doneCh:
		case <-time.After(20 * time.Second):
			t.Fatalf("timed out waiting for Run() #%d to return", i)
		}
	}
	close(errCh)
	for err := range errCh {
		require.NoError(t, err)
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

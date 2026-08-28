//go:build !windows

package supervisor

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/lestrrat-go/server-starter/v2/internal/statefile"
	"github.com/stretchr/testify/require"
)

func TestShutdownForcesStubbornWorkerAndCompletesTeardown(t *testing.T) {
	dir := t.TempDir()
	statusPath := filepath.Join(dir, "status")
	pidPath := filepath.Join(dir, "supervisor.pid")
	socketPath := filepath.Join(dir, "server.sock")
	readyPath := filepath.Join(dir, "ready")

	sd, err := NewStarter(&config{
		command:    buildStubbornWorker(t, dir),
		args:       []string{"ignore-term", readyPath},
		paths:      []string{socketPath},
		pidfile:    pidPath,
		statusfile: statusPath,
		stderr:     io.Discard,
	})
	require.NoError(t, err)
	sd.shutdownGracePeriod = 500 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	ctrl, err := sd.Run(ctx)
	require.NoError(t, err)

	waitForPath(t, readyPath)
	waitForGenerations(t, statusPath, 1)
	status, err := statefile.ReadStatus(ctx, statusPath)
	require.NoError(t, err)
	require.Len(t, status, 1)
	workerPID := 0
	for _, pid := range status {
		workerPID = pid
	}
	require.Positive(t, workerPID)

	cancel()
	started := time.Now()
	select {
	case <-ctrl.Done():
	case <-time.After(3 * time.Second):
		worker, findErr := os.FindProcess(workerPID)
		if findErr == nil {
			_ = worker.Kill()
		}
		t.Fatal("timed out waiting for bounded shutdown")
	}

	require.ErrorIs(t, ctrl.Err(), ErrServerClosed)
	require.GreaterOrEqual(t, time.Since(started), sd.shutdownGracePeriod)
	require.NoFileExists(t, statusPath)
	require.NoFileExists(t, pidPath)
	require.NoFileExists(t, socketPath)

	var waitStatus syscall.WaitStatus
	waited, waitErr := syscall.Wait4(workerPID, &waitStatus, syscall.WNOHANG, nil)
	require.Equal(t, -1, waited)
	require.ErrorIs(t, waitErr, syscall.ECHILD)
}

func TestShutdownStopsWaitingWhenWorkerCannotBeReaped(t *testing.T) {
	const nonexistentPID = 1 << 30
	const gracePeriod = 20 * time.Millisecond

	rs := &runState{
		cfg: &Starter{
			shutdownGracePeriod: gracePeriod,
			stderr:              io.Discard,
		},
		oldWorkers: map[int]int{nonexistentPID: 1},
	}

	started := time.Now()
	rs.shutdownWorkers(syscall.SIGTERM, make(chan processState))

	require.Less(t, time.Since(started), time.Second)
	require.Contains(t, rs.oldWorkers, nonexistentPID)
}

func waitForPath(t *testing.T, path string) {
	t.Helper()

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		} else if !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("failed to stat %s: %s", path, err)
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", path)
}

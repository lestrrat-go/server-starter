//go:build !windows

package control

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/lestrrat-go/server-starter/v2/internal/statefile"
	"github.com/stretchr/testify/require"
)

// TestStopCancelledContext verifies that Stop, given a context that is
// already cancelled, returns promptly instead of waiting out the poll loop.
// It signals only a process this test itself spawned and has already
// waited for, so the pid is guaranteed dead and SIGTERM is a harmless
// ESRCH.
func TestStopCancelledContext(t *testing.T) {
	t.Parallel()

	pidPath := filepath.Join(t.TempDir(), "pid")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result := make(chan error, 1)
	start := time.Now()
	go func() { result <- Stop(ctx, pidPath) }()

	var err error
	select {
	case err = <-result:
	case <-time.After(5 * time.Second):
		t.Fatal("Stop did not return within 5s of an already-cancelled context")
	}
	elapsed := time.Since(start)

	require.Error(t, err)
	require.True(t, errors.Is(err, context.Canceled), "expected context.Canceled, got %v", err)
	require.Less(t, elapsed, 2*time.Second, "Stop did not return promptly on an already-cancelled context")
}

func TestStopDoesNotSignalWhenCancelledDuringPIDOpen(t *testing.T) {
	pidPath := filepath.Join(t.TempDir(), "pid")
	_, signalPath := startControlSignalHelper(t, pidPath)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := stopWithOpenRunningPID(ctx, pidPath, func(path string) (*statefile.RunningPID, error) {
		running, err := statefile.OpenRunningPID(path)
		cancel()
		return running, err
	})

	require.ErrorIs(t, err, context.Canceled)
	require.Never(t, func() bool {
		_, err := os.Stat(signalPath)
		return err == nil
	}, 100*time.Millisecond, 10*time.Millisecond)
}

// TestRestartCancelledContext verifies that Restart, while genuinely
// polling against a status file that never advances, stops promptly when
// its context is cancelled. The child ignores SIGHUP so it survives the
// signal Restart sends it, and this test kills and reaps it explicitly at
// the end.
func TestRestartCancelledContext(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	pidPath := filepath.Join(dir, "pid")
	cmd, _ := startControlSignalHelper(t, pidPath)
	pid := cmd.Process.Pid

	statusPath := filepath.Join(dir, "status")
	// A status file that never advances past generation 1 keeps Restart
	// polling until the context is cancelled.
	require.NoError(t, statefile.WriteStatus(statusPath, map[int]int{1: pid}))

	ctx, cancel := context.WithCancel(context.Background())
	timer := time.AfterFunc(200*time.Millisecond, cancel)
	defer timer.Stop()

	result := make(chan error, 1)
	start := time.Now()
	go func() { result <- Restart(ctx, pidPath, statusPath) }()

	var err error
	select {
	case err = <-result:
	case <-time.After(10 * time.Second):
		t.Fatal("Restart did not return within 10s of context cancellation")
	}
	elapsed := time.Since(start)

	require.Error(t, err)
	require.True(t, errors.Is(err, context.Canceled), "expected context.Canceled, got %v", err)
	require.Less(t, elapsed, 5*time.Second, "Restart did not return well within the old 30s default")
}

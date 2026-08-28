//go:build !windows

package control

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
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

	// context.Background(): this child is a short-lived helper the test
	// owns and waits on directly, not tied to the ctx under test below.
	cmd := exec.CommandContext(context.Background(), "/bin/true")
	require.NoError(t, cmd.Run(), "spawn short-lived child")
	pid := cmd.Process.Pid

	pidPath := filepath.Join(t.TempDir(), "pid")
	require.NoError(t, os.WriteFile(pidPath, []byte(strconv.Itoa(pid)+"\n"), 0644))

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

// TestRestartCancelledContext verifies that Restart, while genuinely
// polling against a status file that never advances, stops promptly when
// its context is cancelled. The child ignores SIGHUP so it survives the
// signal Restart sends it, and this test kills and reaps it explicitly at
// the end.
func TestRestartCancelledContext(t *testing.T) {
	t.Parallel()

	// context.Background(): the test kills and reaps this child itself
	// (see the deferred Kill/Wait below), independent of the ctx under
	// test that gets cancelled 200ms from now.
	cmd := exec.CommandContext(context.Background(), "/bin/sh", "-c", `trap "" HUP; exec sleep 30`)
	require.NoError(t, cmd.Start(), "spawn HUP-immune child")
	pid := cmd.Process.Pid
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()

	dir := t.TempDir()
	pidPath := filepath.Join(dir, "pid")
	require.NoError(t, os.WriteFile(pidPath, []byte(strconv.Itoa(pid)+"\n"), 0644))

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

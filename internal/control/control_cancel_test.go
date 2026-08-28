//go:build !windows

package control

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/lestrrat-go/server-starter/v2/internal/statefile"
	"github.com/stretchr/testify/require"
)

const controlHelperEnv = "SERVER_STARTER_CONTROL_HELPER"
const controlHelperModeEnv = "SERVER_STARTER_CONTROL_HELPER_MODE"

func TestStopSignalsLockedSupervisor(t *testing.T) {
	t.Parallel()

	pidPath := filepath.Join(t.TempDir(), "pid")
	helper := startControlHelper(t, pidPath)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, Stop(ctx, pidPath))
	require.NoError(t, helper.wait())
}

func TestStopSignalsLegacyFlockSupervisor(t *testing.T) {
	pidPath := filepath.Join(t.TempDir(), "pid")
	helper := startControlHelperWithMode(t, pidPath, "legacy")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, Stop(ctx, pidPath))
	require.NoError(t, helper.wait())
}

// TestStopCancelledContext verifies that Stop, given a context that is
// already cancelled, returns promptly instead of waiting out the poll loop.
// The cancelled context is checked before the pid file is opened, so no
// process can be signalled.
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

// TestRestartCancelledContext verifies that Restart, while genuinely
// polling against a status file that never advances, stops promptly when
// its context is cancelled. The child ignores SIGHUP so it survives the
// signal Restart sends it, and this test kills and reaps it explicitly at
// the end.
func TestRestartCancelledContext(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	pidPath := filepath.Join(dir, "pid")
	helper := startControlHelper(t, pidPath)
	pid := helper.pid

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

func TestControlHelperProcess(t *testing.T) {
	path := os.Getenv(controlHelperEnv)
	if path == "" {
		return
	}

	if os.Getenv(controlHelperModeEnv) == "legacy" {
		file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0644)
		require.NoError(t, err)
		defer file.Close()
		require.NoError(t, syscall.Flock(int(file.Fd()), syscall.LOCK_EX))
		_, err = fmt.Fprintf(file, "%d\n", os.Getpid())
		require.NoError(t, err)
		require.NoError(t, file.Sync())
	} else {
		pidFile, err := statefile.Acquire(path)
		require.NoError(t, err)
		defer pidFile.Close()
	}

	signal.Ignore(syscall.SIGHUP)
	defer signal.Reset(syscall.SIGHUP)
	term := make(chan os.Signal, 1)
	signal.Notify(term, syscall.SIGTERM)
	defer signal.Stop(term)

	_, err := fmt.Fprintln(os.Stdout, "ready")
	require.NoError(t, err)
	<-term
}

type controlHelper struct {
	cmd     *exec.Cmd
	pid     int
	waitErr error
	waitOne sync.Once
}

func (h *controlHelper) wait() error {
	h.waitOne.Do(func() {
		h.waitErr = h.cmd.Wait()
	})
	return h.waitErr
}

func startControlHelper(t *testing.T, path string) *controlHelper {
	return startControlHelperWithMode(t, path, "")
}

func startControlHelperWithMode(t *testing.T, path, mode string) *controlHelper {
	t.Helper()

	cmd := exec.CommandContext(context.Background(), os.Args[0], "-test.run=^TestControlHelperProcess$")
	cmd.Env = append(os.Environ(), controlHelperEnv+"="+path)
	if mode != "" {
		cmd.Env = append(cmd.Env, controlHelperModeEnv+"="+mode)
	}
	stdout, err := cmd.StdoutPipe()
	require.NoError(t, err)
	cmd.Stderr = os.Stderr
	require.NoError(t, cmd.Start())
	helper := &controlHelper{cmd: cmd, pid: cmd.Process.Pid}
	t.Cleanup(func() {
		_ = cmd.Process.Signal(syscall.SIGTERM)
		require.NoError(t, helper.wait())
	})

	scanner := bufio.NewScanner(stdout)
	require.True(t, scanner.Scan())
	require.Equal(t, "ready", scanner.Text())
	require.NoError(t, scanner.Err())
	return helper
}

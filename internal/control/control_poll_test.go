//go:build !windows

package control

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
	"time"

	"github.com/lestrrat-go/server-starter/v2/internal/statefile"
	"github.com/stretchr/testify/require"
)

const (
	controlHelperEnv       = "SERVER_STARTER_CONTROL_HELPER"
	controlHelperReadyEnv  = "SERVER_STARTER_CONTROL_HELPER_READY"
	controlHelperSignalEnv = "SERVER_STARTER_CONTROL_HELPER_SIGNAL"
)

func TestControlSignalHelper(t *testing.T) {
	if os.Getenv(controlHelperEnv) != "1" {
		return
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGHUP, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	if err := os.WriteFile(os.Getenv(controlHelperReadyEnv), []byte("ready\n"), 0600); err != nil {
		t.Fatal(err)
	}
	sig := <-sigCh
	if err := os.WriteFile(os.Getenv(controlHelperSignalEnv), []byte(sig.String()+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	select {}
}

func TestStopReturnsPollingOpenError(t *testing.T) {
	cmd, signalPath := startControlSignalHelper(t)

	pidPath := filepath.Join(t.TempDir(), "server.pid")
	f, err := os.OpenFile(pidPath, os.O_CREATE|os.O_RDWR, 0600)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, f.Close()) })
	require.NoError(t, syscall.Flock(int(f.Fd()), syscall.LOCK_EX))
	require.NoError(t, f.Truncate(0))
	_, err = fmt.Fprintf(f, "%d\n", cmd.Process.Pid)
	require.NoError(t, err)
	require.NoError(t, f.Sync())

	// context.Background(): cancel is deferred below, and the result is read
	// before the test returns.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	result := make(chan error, 1)
	go func() { result <- Stop(ctx, pidPath) }()

	waitForPath(t, signalPath)
	require.NoError(t, os.Remove(pidPath))
	require.NoError(t, os.Mkdir(pidPath, 0700))

	err = <-result
	require.ErrorContains(t, err, "failed to open pid file")
	require.NotErrorIs(t, err, context.DeadlineExceeded)
}

func TestRestartReturnsPollingParseError(t *testing.T) {
	cmd, signalPath := startControlSignalHelper(t)

	dir := t.TempDir()
	pidPath := filepath.Join(dir, "server.pid")
	require.NoError(t, os.WriteFile(pidPath, []byte(strconv.Itoa(cmd.Process.Pid)+"\n"), 0600))
	statusPath := filepath.Join(dir, "status")
	require.NoError(t, statefile.WriteStatus(statusPath, map[int]int{1: cmd.Process.Pid}))

	// context.Background(): cancel is deferred below, and the result is read
	// before the test returns.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	result := make(chan error, 1)
	go func() { result <- Restart(ctx, pidPath, statusPath) }()

	waitForPath(t, signalPath)
	require.NoError(t, os.WriteFile(statusPath, []byte("invalid\n"), 0600))

	err := <-result
	require.ErrorContains(t, err, "failed to read status file")
	require.ErrorContains(t, err, "invalid status line")
	require.NotErrorIs(t, err, context.DeadlineExceeded)
}

func startControlSignalHelper(t *testing.T) (*exec.Cmd, string) {
	t.Helper()
	dir := t.TempDir()
	readyPath := filepath.Join(dir, "ready")
	signalPath := filepath.Join(dir, "signal")
	// context.Background(): cleanup below kills and waits for the helper.
	cmd := exec.CommandContext(context.Background(), os.Args[0], "-test.run=^TestControlSignalHelper$")
	cmd.Env = append(os.Environ(),
		controlHelperEnv+"=1",
		controlHelperReadyEnv+"="+readyPath,
		controlHelperSignalEnv+"="+signalPath,
	)
	require.NoError(t, cmd.Start())
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})
	waitForPath(t, readyPath)
	return cmd, signalPath
}

func waitForPath(t *testing.T, path string) {
	t.Helper()
	require.Eventually(t, func() bool {
		_, err := os.Stat(path)
		return err == nil
	}, time.Second, 10*time.Millisecond, "path %q was not created", path)
}

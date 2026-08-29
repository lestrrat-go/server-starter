//go:build !windows

package control

import (
	"context"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
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
	controlHelperPIDEnv    = "SERVER_STARTER_CONTROL_HELPER_PID"
	controlHelperExitEnv   = "SERVER_STARTER_CONTROL_HELPER_EXIT"
)

func TestControlSignalHelper(t *testing.T) {
	if os.Getenv(controlHelperEnv) != "1" {
		return
	}
	var pidFile *statefile.PIDFile
	if path := os.Getenv(controlHelperPIDEnv); path != "" {
		var err error
		pidFile, err = statefile.Acquire(path)
		if err != nil {
			t.Fatal(err)
		}
		defer pidFile.Close()
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
	if os.Getenv(controlHelperExitEnv) == "1" {
		return
	}
	select {}
}

func TestStopKeepsPollingOriginalHandle(t *testing.T) {
	pidPath := filepath.Join(t.TempDir(), "server.pid")
	_, signalPath := startControlSignalHelper(t, pidPath)

	// context.Background(): cancel is deferred below, and the result is read
	// before the test returns.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	result := make(chan error, 1)
	go func() { result <- Stop(ctx, pidPath) }()

	waitForPath(t, signalPath)
	require.NoError(t, os.Remove(pidPath))
	require.NoError(t, os.Mkdir(pidPath, 0700))

	err := <-result
	require.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestRestartReturnsPollingParseError(t *testing.T) {
	dir := t.TempDir()
	pidPath := filepath.Join(dir, "server.pid")
	cmd, signalPath := startControlSignalHelper(t, pidPath)
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

func startControlSignalHelper(t *testing.T, pidPaths ...string) (*exec.Cmd, string) {
	return startControlSignalHelperWithExit(t, false, pidPaths...)
}

func startExitingControlSignalHelper(t *testing.T, pidPath string) (*exec.Cmd, string) {
	return startControlSignalHelperWithExit(t, true, pidPath)
}

func startControlSignalHelperWithExit(t *testing.T, exitOnSignal bool, pidPaths ...string) (*exec.Cmd, string) {
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
	if len(pidPaths) > 0 {
		cmd.Env = append(cmd.Env, controlHelperPIDEnv+"="+pidPaths[0])
	}
	if exitOnSignal {
		cmd.Env = append(cmd.Env, controlHelperExitEnv+"=1")
	}
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

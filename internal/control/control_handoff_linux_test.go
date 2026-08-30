//go:build linux

package control

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const (
	controlHandoffHelperEnv   = "SERVER_STARTER_CONTROL_HANDOFF_HELPER"
	controlHandoffPIDEnv      = "SERVER_STARTER_CONTROL_HANDOFF_PID"
	controlHandoffReadyEnv    = "SERVER_STARTER_CONTROL_HANDOFF_READY"
	controlHandoffStartEnv    = "SERVER_STARTER_CONTROL_HANDOFF_START"
	controlHandoffAcquiredEnv = "SERVER_STARTER_CONTROL_HANDOFF_ACQUIRED"
)

func TestStopReturnsAfterRetainedInodeLockHandoff(t *testing.T) {
	dir := t.TempDir()
	pidPath := filepath.Join(dir, "server.pid")
	_, _ = startExitingControlSignalHelper(t, pidPath)

	readyPath := filepath.Join(dir, "handoff-ready")
	startPath := filepath.Join(dir, "handoff-start")
	acquiredPath := filepath.Join(dir, "handoff-acquired")
	// context.Background(): cleanup below kills and waits for the helper.
	cmd := exec.CommandContext(context.Background(), os.Args[0], "-test.run=^TestRetainedInodeLockHandoffHelper$")
	cmd.Env = append(os.Environ(),
		controlHandoffHelperEnv+"=1",
		controlHandoffPIDEnv+"="+pidPath,
		controlHandoffReadyEnv+"="+readyPath,
		controlHandoffStartEnv+"="+startPath,
		controlHandoffAcquiredEnv+"="+acquiredPath,
	)
	require.NoError(t, cmd.Start())
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})
	waitForPath(t, readyPath)
	require.NoError(t, os.WriteFile(startPath, []byte("start\n"), 0600))

	// context.Background(): cancel is deferred below, and the result is read
	// before the test returns.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	result := make(chan error, 1)
	go func() { result <- Stop(ctx, pidPath) }()

	waitForPath(t, acquiredPath)
	require.NoError(t, <-result)
}

func TestRetainedInodeLockHandoffHelper(t *testing.T) {
	if os.Getenv(controlHandoffHelperEnv) != "1" {
		return
	}
	f, err := os.OpenFile(os.Getenv(controlHandoffPIDEnv), os.O_RDWR, 0)
	require.NoError(t, err)
	defer f.Close()
	require.NoError(t, os.WriteFile(os.Getenv(controlHandoffReadyEnv), []byte("ready\n"), 0600))
	waitForPath(t, os.Getenv(controlHandoffStartEnv))
	require.NoError(t, syscall.Flock(int(f.Fd()), syscall.LOCK_EX))
	lock := syscall.Flock_t{Type: syscall.F_WRLCK, Whence: 0, Start: 0, Len: 0}
	require.NoError(t, syscall.FcntlFlock(f.Fd(), syscall.F_SETLK, &lock))
	require.NoError(t, f.Truncate(0))
	_, err = f.WriteAt([]byte(strconv.Itoa(os.Getpid())+"\n"), 0)
	require.NoError(t, err)
	require.NoError(t, f.Sync())
	require.NoError(t, os.WriteFile(os.Getenv(controlHandoffAcquiredEnv), []byte("acquired\n"), 0600))
	select {}
}

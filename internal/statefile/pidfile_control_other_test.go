//go:build !linux && !windows

package statefile

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAcquiredPIDFileCanBeControlled(t *testing.T) {
	path := filepath.Join(t.TempDir(), "server.pid")
	pidFile, err := Acquire(path)
	require.NoError(t, err)
	defer pidFile.Close()

	running, err := OpenRunningPID(path)
	require.NoError(t, err)
	require.Equal(t, os.Getpid(), running.PID())
	require.NoError(t, running.Close())

	cmd := exec.Command(os.Args[0], "-test.run=^TestPIDFileControlCloseLockHelper$")
	cmd.Env = append(os.Environ(), pidFileControlCloseHelperEnv+"="+path)
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, string(output))
}

const pidFileControlCloseHelperEnv = "SERVER_STARTER_PID_FILE_CONTROL_CLOSE_HELPER"

func TestPIDFileControlCloseLockHelper(t *testing.T) {
	path := os.Getenv(pidFileControlCloseHelperEnv)
	if path == "" {
		return
	}
	contender, err := Acquire(path)
	if contender != nil {
		_ = contender.Close()
	}
	require.ErrorIs(t, err, ErrPIDFileLocked)
}

//go:build !linux && !windows

package statefile

import (
	"os"
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
	defer running.Close()
	require.Equal(t, os.Getpid(), running.PID())
}

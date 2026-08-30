//go:build linux

package control

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/lestrrat-go/server-starter/v2/internal/statefile"
	"github.com/stretchr/testify/require"
)

func TestStopSignalsThroughExecuteOnlyAncestor(t *testing.T) {
	dir := t.TempDir()
	ancestor := filepath.Join(dir, "supervisor")
	require.NoError(t, os.Mkdir(ancestor, 0700))
	pidPath := filepath.Join(ancestor, "server.pid")
	_, signalPath := startExitingControlSignalHelper(t, pidPath)
	require.NoError(t, os.Chmod(ancestor, 0100))
	t.Cleanup(func() { _ = os.Chmod(ancestor, 0700) })

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	require.NoError(t, Stop(ctx, pidPath))

	signal, err := os.ReadFile(signalPath)
	require.NoError(t, err)
	require.Equal(t, "terminated\n", string(signal))
}

func TestRestartSignalsThroughExecuteOnlyAncestor(t *testing.T) {
	dir := t.TempDir()
	ancestor := filepath.Join(dir, "supervisor")
	require.NoError(t, os.Mkdir(ancestor, 0700))
	pidPath := filepath.Join(ancestor, "server.pid")
	statusPath := filepath.Join(ancestor, "status")
	cmd, signalPath := startControlSignalHelper(t, pidPath)
	require.NoError(t, statefile.WriteStatus(statusPath, map[int]int{1: cmd.Process.Pid}))
	statusFile, err := os.OpenFile(statusPath, os.O_WRONLY, 0)
	require.NoError(t, err)
	t.Cleanup(func() { _ = statusFile.Close() })
	require.NoError(t, os.Chmod(ancestor, 0100))
	t.Cleanup(func() { _ = os.Chmod(ancestor, 0700) })

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	result := make(chan error, 1)
	go func() { result <- Restart(ctx, pidPath, statusPath) }()

	waitForPath(t, signalPath)
	require.NoError(t, statusFile.Truncate(0))
	_, err = statusFile.WriteAt([]byte("2:1\n"), 0)
	require.NoError(t, err)
	require.NoError(t, statusFile.Sync())
	require.NoError(t, <-result)

	signal, err := os.ReadFile(signalPath)
	require.NoError(t, err)
	require.Equal(t, "hangup\n", string(signal))
}

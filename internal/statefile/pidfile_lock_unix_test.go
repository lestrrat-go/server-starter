//go:build !windows

package statefile_test

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"

	"github.com/lestrrat-go/server-starter/v2/internal/statefile"
	"github.com/stretchr/testify/require"
)

const pidLockHelperEnv = "SERVER_STARTER_PID_LOCK_HELPER"

func TestReadPIDRequiresLiveMatchingLockOwner(t *testing.T) {
	t.Run("accepts the supervisor that owns the lock", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "server.pid")
		pid := startPIDLockHelper(t, path)

		got, err := statefile.ReadPID(path)
		require.NoError(t, err)
		require.Equal(t, pid, got)
	})

	t.Run("rejects an unlocked stale file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "server.pid")
		require.NoError(t, os.WriteFile(path, fmt.Appendf(nil, "%d\n", os.Getpid()), 0644))

		_, err := statefile.ReadPID(path)
		require.ErrorContains(t, err, "not locked by a running supervisor")
	})

	t.Run("rejects a replaced pid that differs from the lock owner", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "server.pid")
		ownerPID := startPIDLockHelper(t, path)
		replacementPID := os.Getpid()
		require.NotEqual(t, ownerPID, replacementPID)
		require.NoError(t, os.WriteFile(path, fmt.Appendf(nil, "%d\n", replacementPID), 0644))

		_, err := statefile.ReadPID(path)
		require.ErrorContains(t, err, "does not match lock owner")
	})
}

func TestPIDLockHelper(t *testing.T) {
	if os.Getenv(pidLockHelperEnv) == "" {
		return
	}

	path := os.Getenv(pidLockHelperEnv)
	pidFile, err := statefile.Acquire(path)
	require.NoError(t, err)
	defer pidFile.Close()

	term := make(chan os.Signal, 1)
	signal.Notify(term, syscall.SIGTERM)
	defer signal.Stop(term)

	_, err = fmt.Fprintln(os.Stdout, "ready")
	require.NoError(t, err)
	<-term
}

func startPIDLockHelper(t *testing.T, path string) int {
	t.Helper()

	cmd := exec.CommandContext(context.Background(), os.Args[0], "-test.run=^TestPIDLockHelper$")
	cmd.Env = append(os.Environ(), pidLockHelperEnv+"="+path)
	stdout, err := cmd.StdoutPipe()
	require.NoError(t, err)
	cmd.Stderr = os.Stderr
	require.NoError(t, cmd.Start())
	t.Cleanup(func() {
		_ = cmd.Process.Signal(syscall.SIGTERM)
		require.NoError(t, cmd.Wait())
	})

	scanner := bufio.NewScanner(stdout)
	require.True(t, scanner.Scan())
	require.Equal(t, "ready", scanner.Text())
	require.NoError(t, scanner.Err())

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	pid, err := strconv.Atoi(string(bytes.TrimSpace(data)))
	require.NoError(t, err)
	return pid
}

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
	"time"

	"github.com/lestrrat-go/server-starter/v2/internal/statefile"
	"github.com/stretchr/testify/require"
)

const pidLockHelperEnv = "SERVER_STARTER_PID_LOCK_HELPER"
const pidLockHelperModeEnv = "SERVER_STARTER_PID_LOCK_HELPER_MODE"

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

	t.Run("accepts a legacy flock owner", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "server.pid")
		pid := startLegacyPIDLockHelper(t, path)

		got, err := statefile.ReadPID(path)
		require.NoError(t, err)
		require.Equal(t, pid, got)
	})

	t.Run("rejects a legacy flock whose recorded pid does not own it", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "server.pid")
		ownerPID := startLegacyPIDLockHelper(t, path)
		replacementPID := os.Getpid()
		require.NotEqual(t, ownerPID, replacementPID)
		require.NoError(t, os.WriteFile(path, fmt.Appendf(nil, "%d\n", replacementPID), 0644))

		_, err := statefile.ReadPID(path)
		require.Error(t, err)
	})

	t.Run("rejects a locked pid file moved from another path", func(t *testing.T) {
		dir := t.TempDir()
		firstPath := filepath.Join(dir, "first.pid")
		secondPath := filepath.Join(dir, "second.pid")
		startPIDLockHelper(t, firstPath)
		startPIDLockHelper(t, secondPath)

		require.NoError(t, os.Rename(secondPath, firstPath))
		_, err := statefile.ReadPID(firstPath)
		require.ErrorContains(t, err, "different path")
	})
}

func TestAcquireBlocksWhileSameProcessHoldsPIDFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "server.pid")
	first, err := statefile.Acquire(path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = first.Close() })

	type result struct {
		pidFile *statefile.PIDFile
		err     error
	}
	acquired := make(chan result, 1)
	go func() {
		pidFile, err := statefile.Acquire(path)
		acquired <- result{pidFile: pidFile, err: err}
	}()

	select {
	case second := <-acquired:
		if second.pidFile != nil {
			require.NoError(t, second.pidFile.Close())
		}
		require.Failf(t, "second acquire returned", "error = %v", second.err)
	case <-time.After(200 * time.Millisecond):
	}

	require.NoError(t, first.Close())
	select {
	case second := <-acquired:
		require.NoError(t, second.err)
		require.NoError(t, second.pidFile.Close())
	case <-time.After(5 * time.Second):
		require.Fail(t, "second acquire remained blocked after the first file closed")
	}
}

func TestPIDLockHelper(t *testing.T) {
	if os.Getenv(pidLockHelperEnv) == "" {
		return
	}

	path := os.Getenv(pidLockHelperEnv)
	if os.Getenv(pidLockHelperModeEnv) == "legacy" {
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

	term := make(chan os.Signal, 1)
	signal.Notify(term, syscall.SIGTERM)
	defer signal.Stop(term)

	_, err := fmt.Fprintln(os.Stdout, "ready")
	require.NoError(t, err)
	<-term
}

func startPIDLockHelper(t *testing.T, path string) int {
	return startPIDLockHelperWithMode(t, path, "")
}

func startLegacyPIDLockHelper(t *testing.T, path string) int {
	return startPIDLockHelperWithMode(t, path, "legacy")
}

func startPIDLockHelperWithMode(t *testing.T, path, mode string) int {
	t.Helper()

	cmd := exec.CommandContext(context.Background(), os.Args[0], "-test.run=^TestPIDLockHelper$")
	cmd.Env = append(os.Environ(), pidLockHelperEnv+"="+path)
	if mode != "" {
		cmd.Env = append(cmd.Env, pidLockHelperModeEnv+"="+mode)
	}
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

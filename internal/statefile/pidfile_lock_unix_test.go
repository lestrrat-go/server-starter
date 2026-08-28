//go:build !windows

package statefile_test

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"syscall"
	"testing"

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
		if runtime.GOOS == "linux" {
			require.NoError(t, err)
			require.Equal(t, pid, got)
			return
		}
		require.ErrorContains(t, err, "cannot be attributed to a process")
		require.Zero(t, got)
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

	t.Run("rejects separate record and flock owners", func(t *testing.T) {
		if runtime.GOOS != "linux" {
			t.Skip("Linux exposes BSD flock owners through /proc/locks")
		}

		path := filepath.Join(t.TempDir(), "server.pid")
		flockPID := startLegacyPIDLockHelper(t, path)
		recordPID := startRecordPIDLockHelper(t, path)
		require.NotEqual(t, flockPID, recordPID)

		_, err := statefile.ReadPID(path)
		require.ErrorContains(t, err, "record lock owner")
	})

	t.Run("rejects a record lock without a BSD flock", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "server.pid")
		startRecordPIDLockHelper(t, path)

		_, err := statefile.ReadPID(path)
		require.ErrorContains(t, err, "could not be verified as the BSD flock owner")
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

func TestReadPIDAllowsDifferentPIDFileOwner(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("changing pid-file ownership requires root")
	}

	path := filepath.Join(t.TempDir(), "server.pid")
	pid := startPIDLockHelper(t, path)
	require.NoError(t, os.Chown(path, 1, -1))

	got, err := statefile.ReadPID(path)
	require.NoError(t, err)
	require.Equal(t, pid, got)
}

func TestAcquireAllowsDifferentPIDFileOwner(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("changing pid-file ownership requires root")
	}

	path := filepath.Join(t.TempDir(), "server.pid")
	require.NoError(t, os.WriteFile(path, nil, 0644))
	require.NoError(t, os.Chown(path, 1, -1))

	pidFile, err := statefile.Acquire(path)
	require.NoError(t, err)
	require.NoError(t, pidFile.Close())
}

func TestPIDLockHelper(t *testing.T) {
	if os.Getenv(pidLockHelperEnv) == "" {
		return
	}

	path := os.Getenv(pidLockHelperEnv)
	switch os.Getenv(pidLockHelperModeEnv) {
	case "legacy", "record":
		file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0644)
		require.NoError(t, err)
		defer file.Close()
		if os.Getenv(pidLockHelperModeEnv) == "legacy" {
			require.NoError(t, syscall.Flock(int(file.Fd()), syscall.LOCK_EX))
		} else {
			lock := testPathRecordLock(path)
			require.NoError(t, syscall.FcntlFlock(file.Fd(), syscall.F_SETLK, &lock))
		}
		_, err = fmt.Fprintf(file, "%d\n", os.Getpid())
		require.NoError(t, err)
		require.NoError(t, file.Sync())
	default:
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

func startRecordPIDLockHelper(t *testing.T, path string) int {
	return startPIDLockHelperWithMode(t, path, "record")
}

func testPathRecordLock(path string) syscall.Flock_t {
	absPath, err := filepath.Abs(path)
	if err != nil {
		panic(err)
	}
	digest := sha256.Sum256([]byte(filepath.Clean(absPath)))
	start := int64(binary.BigEndian.Uint64(digest[:8]) & (^uint64(0) >> 1))
	if start == 0 {
		start = 1
	}
	return syscall.Flock_t{Type: syscall.F_WRLCK, Whence: 0, Start: start, Len: 1}
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

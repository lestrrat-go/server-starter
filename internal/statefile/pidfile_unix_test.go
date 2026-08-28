//go:build !windows

package statefile

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestOpenRunningPIDRejectsSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.pid")
	path := filepath.Join(dir, "server.pid")
	require.NoError(t, os.WriteFile(target, fmt.Appendf(nil, "%d\n", os.Getpid()), 0600))
	require.NoError(t, os.Symlink(target, path))

	running, err := OpenRunningPID(path)
	require.Error(t, err)
	require.Nil(t, running)
}

func TestOpenRunningPIDRejectsFIFOWithoutBlocking(t *testing.T) {
	path := filepath.Join(t.TempDir(), "server.pid")
	require.NoError(t, syscall.Mkfifo(path, 0600))

	result := make(chan error, 1)
	go func() {
		_, err := OpenRunningPID(path)
		result <- err
	}()

	select {
	case err := <-result:
		require.ErrorContains(t, err, "is not a regular file")
	case <-time.After(time.Second):
		require.Fail(t, "OpenRunningPID blocked while opening a FIFO")
	}
}

func TestOpenRunningPIDRejectsOversizedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "server.pid")
	require.NoError(t, os.WriteFile(path, make([]byte, maxPIDFileSize+1), 0600))

	running, err := OpenRunningPID(path)
	require.ErrorContains(t, err, "is too large")
	require.Nil(t, running)
}

func TestAcquireRejectsSymlinkWithoutChangingTarget(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	path := filepath.Join(dir, "server.pid")
	require.NoError(t, os.WriteFile(target, []byte("keep me"), 0600))
	require.NoError(t, os.Symlink(target, path))

	pidFile, err := Acquire(path)
	require.Error(t, err)
	require.Nil(t, pidFile)

	data, err := os.ReadFile(target)
	require.NoError(t, err)
	require.Equal(t, "keep me", string(data))
}

func TestAcquireRejectsHardLinkWithoutChangingTarget(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	path := filepath.Join(dir, "server.pid")
	require.NoError(t, os.WriteFile(target, []byte("keep me"), 0600))
	require.NoError(t, os.Link(target, path))

	pidFile, err := Acquire(path)
	require.Error(t, err)
	require.Nil(t, pidFile)

	data, err := os.ReadFile(target)
	require.NoError(t, err)
	require.Equal(t, "keep me", string(data))
}

func TestAcquireRejectsHardLinkAddedWhileWaitingForLock(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "server.pid")
	linkPath := filepath.Join(dir, "server.pid.link")
	require.NoError(t, os.WriteFile(path, []byte("keep me"), 0600))

	lockHolder, err := os.OpenFile(path, os.O_RDWR, 0)
	require.NoError(t, err)
	defer lockHolder.Close()
	require.NoError(t, syscall.Flock(int(lockHolder.Fd()), syscall.LOCK_EX))

	lockAttempted := make(chan struct{})
	type acquireResult struct {
		pidFile *PIDFile
		err     error
	}
	result := make(chan acquireResult, 1)
	go func() {
		pidFile, acquireErr := acquire(path, func(f *os.File) error {
			close(lockAttempted)
			return syscall.Flock(int(f.Fd()), syscall.LOCK_EX)
		})
		result <- acquireResult{pidFile: pidFile, err: acquireErr}
	}()

	<-lockAttempted
	require.NoError(t, os.Link(path, linkPath))
	require.NoError(t, lockHolder.Close())

	acquired := <-result
	require.ErrorContains(t, acquired.err, "has 2 hard links, expected one")
	require.Nil(t, acquired.pidFile)

	data, err := os.ReadFile(linkPath)
	require.NoError(t, err)
	require.Equal(t, "keep me", string(data))
}

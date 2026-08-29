//go:build !windows

package statefile

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/stretchr/testify/require"
)

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

func TestPreparedRunningPIDPathRetainsResolvedParent(t *testing.T) {
	dir := t.TempDir()
	parentA := filepath.Join(dir, "supervisor-a")
	parentB := filepath.Join(dir, "supervisor-b")
	pidParent := filepath.Join(dir, "current")
	require.NoError(t, os.Mkdir(parentA, 0700))
	require.NoError(t, os.Mkdir(parentB, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(parentA, "server.pid"), []byte("101\n"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(parentB, "server.pid"), []byte("202\n"), 0600))
	require.NoError(t, os.Symlink(parentA, pidParent))

	preparedPath, err := prepareRunningPIDPath(filepath.Join(pidParent, "server.pid"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = preparedPath.close() })
	require.NoError(t, os.Remove(pidParent))
	require.NoError(t, os.Symlink(parentB, pidParent))

	f, err := preparedPath.open()
	require.NoError(t, err)
	t.Cleanup(func() { _ = f.Close() })
	data, err := os.ReadFile(f.Name())
	require.NoError(t, err)
	require.Equal(t, "202\n", string(data))

	data = make([]byte, len("101\n"))
	n, err := readPIDText(f, data)
	require.NoError(t, err)
	require.Equal(t, "101\n", string(data[:n]))
	require.NoError(t, preparedPath.validate(f))
}

func TestPrepareRunningPIDPathRejectsReplaceableAncestor(t *testing.T) {
	dir := t.TempDir()
	replaceable := filepath.Join(dir, "replaceable")
	parent := filepath.Join(replaceable, "supervisor")
	require.NoError(t, os.Mkdir(replaceable, 0777))
	require.NoError(t, os.Chmod(replaceable, 0777))
	require.NoError(t, os.Mkdir(parent, 0700))

	preparedPath, err := prepareRunningPIDPath(filepath.Join(parent, "server.pid"))
	require.ErrorContains(t, err, "allows untrusted replacement")
	require.Nil(t, preparedPath)
}

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

func TestPrepareRunningPIDPathRejectsSymlinkedAncestor(t *testing.T) {
	dir := t.TempDir()
	parent := filepath.Join(dir, "supervisor")
	pidParent := filepath.Join(dir, "current")
	require.NoError(t, os.Mkdir(parent, 0700))
	require.NoError(t, os.Symlink(parent, pidParent))

	for _, path := range []string{
		filepath.Join(pidParent, "server.pid"),
		pidParent + string(os.PathSeparator) + ".." + string(os.PathSeparator) + "server.pid",
	} {
		preparedPath, err := prepareRunningPIDPath(path)
		require.ErrorContains(t, err, "contains symbolic link component")
		require.Nil(t, preparedPath)
	}
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

func TestRootAnchoredDirectoryPathSupportsExecuteOnlyAncestor(t *testing.T) {
	dir := t.TempDir()
	ancestor := filepath.Join(dir, "supervisor")
	require.NoError(t, os.Mkdir(ancestor, 0700))
	path := filepath.Join(ancestor, "server.pid")
	require.NoError(t, os.WriteFile(path, []byte("1\n"), 0600))
	require.NoError(t, os.Chmod(ancestor, 0100))
	t.Cleanup(func() { _ = os.Chmod(ancestor, 0700) })

	parent, relativeParent, err := openRootAnchoredDirectoryPath(ancestor, path)
	require.NoError(t, err)
	prepared := &runningPIDPath{
		path:   path,
		parent: parent,
		name:   filepath.Join(relativeParent, filepath.Base(path)),
	}
	t.Cleanup(func() { _ = prepared.close() })

	f, err := prepared.open()
	require.NoError(t, err)
	require.NoError(t, f.Close())
}

func TestRootAnchoredDirectoryPathRejectsSymlinkedAncestor(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	link := filepath.Join(dir, "current")
	require.NoError(t, os.Mkdir(target, 0700))
	require.NoError(t, os.Symlink(target, link))

	parent, _, err := openRootAnchoredDirectoryPath(link, filepath.Join(link, "server.pid"))
	require.ErrorContains(t, err, "contains symbolic link component")
	require.Nil(t, parent)
}

func TestRootAnchoredDirectoryPathRejectsReplaceableAncestor(t *testing.T) {
	dir := t.TempDir()
	replaceable := filepath.Join(dir, "replaceable")
	parentPath := filepath.Join(replaceable, "supervisor")
	require.NoError(t, os.Mkdir(replaceable, 0777))
	require.NoError(t, os.Chmod(replaceable, 0777))
	require.NoError(t, os.Mkdir(parentPath, 0700))

	parent, _, err := openRootAnchoredDirectoryPath(parentPath, filepath.Join(parentPath, "server.pid"))
	require.ErrorContains(t, err, "allows untrusted replacement")
	require.Nil(t, parent)
}

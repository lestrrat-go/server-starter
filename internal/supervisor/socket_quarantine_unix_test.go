//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package supervisor

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCreatePrivateDirAtRejectsReplacementDirectory(t *testing.T) {
	parentPath := t.TempDir()
	parent, err := os.Open(parentPath)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, parent.Close())
	})

	const quarantineName = "quarantine"
	openReached := make(chan struct{})
	continueOpen := make(chan struct{}, 1)
	result := make(chan error, 1)
	t.Cleanup(func() {
		select {
		case continueOpen <- struct{}{}:
		default:
		}
	})
	go func() {
		quarantine, createErr := createPrivateDirAtWithOpen(
			parent,
			quarantineName,
			func(dir *os.File, name string) (*os.File, error) {
				close(openReached)
				<-continueOpen
				return openPrivateDirAt(dir, name)
			},
		)
		if quarantine != nil {
			_ = quarantine.Close()
		}
		result <- createErr
	}()

	<-openReached
	originalPath := filepath.Join(parentPath, "original-quarantine")
	require.NoError(t, os.Rename(filepath.Join(parentPath, quarantineName), originalPath))
	replacementPath := filepath.Join(parentPath, quarantineName)
	require.NoError(t, os.Mkdir(replacementPath, 0700))
	continueOpen <- struct{}{}

	require.ErrorContains(t, <-result, "changed between creation and open")
	info, err := os.Stat(originalPath)
	require.NoError(t, err)
	require.True(t, info.IsDir())
	info, err = os.Stat(replacementPath)
	require.NoError(t, err)
	require.True(t, info.IsDir())
}

func TestCreatePrivateDirAtOpenFailurePreservesReplacementDirectory(t *testing.T) {
	parentPath := t.TempDir()
	parent, err := os.Open(parentPath)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, parent.Close()) })

	const quarantineName = "quarantine"
	replacementPath := filepath.Join(parentPath, quarantineName)
	injectedErr := errors.New("injected open failure")
	quarantine, err := createPrivateDirAtWithOpen(
		parent,
		quarantineName,
		func(_ *os.File, _ string) (*os.File, error) {
			require.NoError(t, os.Rename(replacementPath, filepath.Join(parentPath, "original-quarantine")))
			require.NoError(t, os.Mkdir(replacementPath, 0700))
			return nil, injectedErr
		},
	)
	require.Nil(t, quarantine)
	require.ErrorIs(t, err, injectedErr)
	info, err := os.Stat(replacementPath)
	require.NoError(t, err)
	require.True(t, info.IsDir())
}

func TestRemoveExistingUnixSocketRejectsSocketReplacement(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "server.sock")
	replacementPath := filepath.Join(dir, "replacement.sock")
	createStaleUnixSocket(t, path)
	createStaleUnixSocket(t, replacementPath)

	err := removeExistingUnixSocketWithMove(path, func(
		oldDir *os.File,
		oldName string,
		newDir *os.File,
		newName string,
	) error {
		require.NoError(t, os.Rename(replacementPath, path))
		return moveToQuarantineAt(oldDir, oldName, newDir, newName)
	})
	require.ErrorContains(t, err, "changed during preparation")
	info, err := os.Lstat(path)
	require.NoError(t, err)
	require.NotZero(t, info.Mode()&os.ModeSocket)
}

func TestRemoveExistingUnixSocketPinsSelectedSocket(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "server.sock")
	createStaleUnixSocket(t, path)

	parent, err := os.Open(dir)
	require.NoError(t, err)
	selected, err := pathIdentityAt(parent, filepath.Base(path))
	require.NoError(t, err)
	require.NoError(t, parent.Close())

	err = removeExistingUnixSocketWithMove(path, func(
		oldDir *os.File,
		oldName string,
		newDir *os.File,
		newName string,
	) error {
		pinned, inspectErr := pathIdentityAt(newDir, pinnedSocketName)
		require.NoError(t, inspectErr)
		require.True(t, samePathIdentity(selected, pinned))

		require.NoError(t, os.Remove(path))
		createStaleUnixSocket(t, path)
		replacement, inspectErr := pathIdentityAt(oldDir, oldName)
		require.NoError(t, inspectErr)
		require.False(t, samePathIdentity(pinned, replacement))
		return moveToQuarantineAt(oldDir, oldName, newDir, newName)
	})
	require.ErrorContains(t, err, "changed during preparation")
	info, err := os.Lstat(path)
	require.NoError(t, err)
	require.NotZero(t, info.Mode()&os.ModeSocket)
}

func createStaleUnixSocket(t *testing.T, path string) {
	t.Helper()
	listener, err := net.ListenUnix(unixNetwork, &net.UnixAddr{Name: path, Net: unixNetwork})
	require.NoError(t, err)
	listener.SetUnlinkOnClose(false)
	require.NoError(t, listener.Close())
}

func TestRenameNoReplaceByLinkAtPreservesDestination(t *testing.T) {
	root := t.TempDir()
	quarantinePath := filepath.Join(root, "quarantine")
	destinationPath := filepath.Join(root, "destination")
	require.NoError(t, os.Mkdir(quarantinePath, 0700))
	require.NoError(t, os.Mkdir(destinationPath, 0700))

	quarantine, err := os.Open(quarantinePath)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, quarantine.Close())
	})
	destination, err := os.Open(destinationPath)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, destination.Close())
	})

	const sourceName = "socket"
	const destinationName = "server.sock"
	sourceContents := []byte("preserved source")
	destinationContents := []byte("preserved destination")
	require.NoError(t, os.WriteFile(filepath.Join(quarantinePath, sourceName), sourceContents, 0600))
	require.NoError(t, os.WriteFile(filepath.Join(destinationPath, destinationName), destinationContents, 0600))

	err = renameNoReplaceByLinkAt(quarantine, sourceName, destination, destinationName)
	require.Error(t, err)
	gotSource, readErr := os.ReadFile(filepath.Join(quarantinePath, sourceName))
	require.NoError(t, readErr)
	require.Equal(t, sourceContents, gotSource)
	gotDestination, readErr := os.ReadFile(filepath.Join(destinationPath, destinationName))
	require.NoError(t, readErr)
	require.Equal(t, destinationContents, gotDestination)

	require.NoError(t, os.Remove(filepath.Join(destinationPath, destinationName)))
	require.NoError(t, renameNoReplaceByLinkAt(quarantine, sourceName, destination, destinationName))
	_, err = os.Lstat(filepath.Join(quarantinePath, sourceName))
	require.ErrorIs(t, err, os.ErrNotExist)
	gotDestination, err = os.ReadFile(filepath.Join(destinationPath, destinationName))
	require.NoError(t, err)
	require.Equal(t, sourceContents, gotDestination)
}

func TestRenameNoReplaceByLinkAtPreservesDirectoryWhenAtomicRestoreIsUnavailable(t *testing.T) {
	root := t.TempDir()
	quarantinePath := filepath.Join(root, "quarantine")
	destinationPath := filepath.Join(root, "destination")
	const sourceName = "quarantined-directory"
	const destinationName = "restored-directory"
	require.NoError(t, os.Mkdir(quarantinePath, 0700))
	require.NoError(t, os.Mkdir(destinationPath, 0700))
	require.NoError(t, os.Mkdir(filepath.Join(quarantinePath, sourceName), 0700))

	quarantine, err := os.Open(quarantinePath)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, quarantine.Close()) })
	destination, err := os.Open(destinationPath)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, destination.Close()) })

	err = renameNoReplaceByLinkAt(quarantine, sourceName, destination, destinationName)
	require.ErrorIs(t, err, errRenameNoReplaceUnsupported)
	info, err := os.Stat(filepath.Join(quarantinePath, sourceName))
	require.NoError(t, err)
	require.True(t, info.IsDir())
	_, err = os.Lstat(filepath.Join(destinationPath, destinationName))
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestRenameNoReplaceByLinkAtUsesAnchoredDirectories(t *testing.T) {
	tests := map[string]struct {
		replaceQuarantine  bool
		replaceDestination bool
	}{
		"quarantine pathname":  {replaceQuarantine: true},
		"destination pathname": {replaceDestination: true},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			quarantinePath := filepath.Join(root, "quarantine")
			destinationPath := filepath.Join(root, "destination")
			require.NoError(t, os.Mkdir(quarantinePath, 0700))
			require.NoError(t, os.Mkdir(destinationPath, 0700))
			require.NoError(t, os.WriteFile(filepath.Join(quarantinePath, "socket"), []byte("original"), 0600))

			quarantine, err := os.Open(quarantinePath)
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, quarantine.Close()) })
			destination, err := os.Open(destinationPath)
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, destination.Close()) })

			anchoredDestinationPath := destinationPath
			if test.replaceQuarantine {
				require.NoError(t, os.Rename(quarantinePath, quarantinePath+"-anchored"))
				require.NoError(t, os.Mkdir(quarantinePath, 0700))
			}
			if test.replaceDestination {
				anchoredDestinationPath = destinationPath + "-anchored"
				require.NoError(t, os.Rename(destinationPath, anchoredDestinationPath))
				require.NoError(t, os.Mkdir(destinationPath, 0700))
			}

			require.NoError(t, renameNoReplaceByLinkAt(quarantine, "socket", destination, "server.sock"))
			contents, err := os.ReadFile(filepath.Join(anchoredDestinationPath, "server.sock"))
			require.NoError(t, err)
			require.Equal(t, []byte("original"), contents)
		})
	}
}

func TestRenameNoReplaceByLinkAtPreservesSourceThroughDestinationRace(t *testing.T) {
	root := t.TempDir()
	quarantinePath := filepath.Join(root, "quarantine")
	destinationPath := filepath.Join(root, "destination")
	require.NoError(t, os.Mkdir(quarantinePath, 0700))
	require.NoError(t, os.Mkdir(destinationPath, 0700))
	sourcePath := filepath.Join(quarantinePath, "socket")
	restoredPath := filepath.Join(destinationPath, "server.sock")
	require.NoError(t, os.WriteFile(sourcePath, []byte("original"), 0600))

	quarantine, err := os.Open(quarantinePath)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, quarantine.Close()) })
	destination, err := os.Open(destinationPath)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, destination.Close()) })

	err = renameNoReplaceByLinkAtWithBeforeUnlink(
		quarantine,
		"socket",
		destination,
		"server.sock",
		func() error {
			require.NoError(t, os.Rename(restoredPath, restoredPath+"-original"))
			return os.WriteFile(restoredPath, []byte("replacement"), 0600)
		},
	)
	require.ErrorContains(t, err, "changed before source removal")
	contents, err := os.ReadFile(sourcePath)
	require.NoError(t, err)
	require.Equal(t, []byte("original"), contents)
	contents, err = os.ReadFile(restoredPath)
	require.NoError(t, err)
	require.Equal(t, []byte("replacement"), contents)
}

func TestRemoveAtPreservesReplacementEntry(t *testing.T) {
	root := t.TempDir()
	dir, err := os.Open(root)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, dir.Close()) })
	path := filepath.Join(root, "socket")
	require.NoError(t, os.WriteFile(path, []byte("original"), 0600))
	selected, err := pathIdentityAt(dir, "socket")
	require.NoError(t, err)
	require.NoError(t, os.Rename(path, path+"-original"))
	require.NoError(t, os.WriteFile(path, []byte("replacement"), 0600))

	require.ErrorContains(t, removeAt(dir, "socket", selected), "changed before removal")
	contents, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, []byte("replacement"), contents)
}

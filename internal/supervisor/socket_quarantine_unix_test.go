//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package supervisor

import (
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
	require.NoError(t, os.Chmod(replacementPath, 0777))
	continueOpen <- struct{}{}

	require.ErrorContains(t, <-result, "permissions")
	info, err := os.Stat(originalPath)
	require.NoError(t, err)
	require.True(t, info.IsDir())
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

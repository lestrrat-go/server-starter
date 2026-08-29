//go:build linux

package supervisor

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

func TestCreatePrivateDirAtRejectsReusedDirectoryIdentity(t *testing.T) {
	parentPath := t.TempDir()
	parent, err := os.Open(parentPath)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, parent.Close()) })

	const quarantineName = "quarantine"
	quarantinePath := filepath.Join(parentPath, quarantineName)
	var reused bool
	quarantine, err := createPrivateDirAtWithOpen(
		parent,
		quarantineName,
		func(dir *os.File, name string) (*os.File, error) {
			created, inspectErr := pathIdentityAt(dir, name)
			require.NoError(t, inspectErr)
			require.NoError(t, os.Remove(quarantinePath))
			for range 1000 {
				require.NoError(t, os.Mkdir(quarantinePath, 0700))
				replacement, replacementErr := pathIdentityAt(dir, name)
				require.NoError(t, replacementErr)
				if samePathIdentity(created, replacement) {
					reused = true
					break
				}
				require.NoError(t, os.Remove(quarantinePath))
			}
			if !reused {
				return nil, errors.New("filesystem did not reuse the directory identity")
			}
			return openPrivateDirAt(dir, name)
		},
	)
	if quarantine != nil {
		require.NoError(t, quarantine.Close())
	}
	if !reused {
		t.Skip("filesystem did not reuse the directory identity")
	}
	require.ErrorContains(t, err, "changed between creation and open")
	info, err := os.Stat(quarantinePath)
	require.NoError(t, err)
	require.True(t, info.IsDir())
}

func TestRemoveExistingUnixSocketProtectsQuarantineFromNameExchange(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "server.sock")
	addr, err := net.ResolveUnixAddr(unixNetwork, path)
	require.NoError(t, err)
	listener, err := net.ListenUnix(unixNetwork, addr)
	require.NoError(t, err)
	listener.SetUnlinkOnClose(false)
	require.NoError(t, listener.Close())

	contents := []byte("replacement")
	var exchangeErr error
	var replacementPath string
	err = removeExistingUnixSocketWithMove(path, func(
		oldDir *os.File,
		oldName string,
		newDir *os.File,
		newName string,
	) error {
		require.NoError(t, renameNoReplaceAt(oldDir, oldName, newDir, newName))
		info, statErr := newDir.Stat()
		require.NoError(t, statErr)
		require.Equal(t, os.FileMode(0700), info.Mode().Perm())
		require.NoError(t, os.WriteFile(path, contents, 0600))
		identity, inspectErr := pathIdentityAt(newDir, newName)
		require.NoError(t, inspectErr)
		require.True(t, identity.isSocket())

		replacementPath = filepath.Join(dir, filepath.Base(newDir.Name()))
		exchangeErr = unix.Renameat2(
			int(oldDir.Fd()),
			oldName,
			int(oldDir.Fd()),
			filepath.Base(newDir.Name()),
			unix.RENAME_EXCHANGE,
		)
		return nil
	})
	require.ErrorContains(t, err, "remove quarantine directory")
	require.NoError(t, exchangeErr)

	got, err := os.ReadFile(replacementPath)
	require.NoError(t, err)
	require.Equal(t, contents, got)
	info, err := os.Stat(path)
	require.NoError(t, err)
	require.True(t, info.IsDir())
}

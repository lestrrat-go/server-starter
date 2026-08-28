//go:build linux

package supervisor

import (
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

func TestRemoveExistingUnixSocketProtectsQuarantineFromNameExchange(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "server.sock")
	addr, err := net.ResolveUnixAddr("unix", path)
	require.NoError(t, err)
	listener, err := net.ListenUnix("unix", addr)
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
		isSocket, inspectErr := pathIsSocketAt(newDir, newName)
		require.NoError(t, inspectErr)
		require.True(t, isSocket)

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

package supervisor

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUnsupportedRenameNoReplaceLeavesSourceUntouched(t *testing.T) {
	dirPath := t.TempDir()
	dir, err := os.Open(dirPath)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, dir.Close())
	})

	oldName := "server.sock"
	newName := "quarantine"
	contents := []byte("replacement")
	require.NoError(t, os.WriteFile(filepath.Join(dirPath, oldName), contents, 0600))

	err = unsupportedRenameNoReplaceAt(dir, oldName, newName)
	require.ErrorIs(t, err, errRenameNoReplaceUnsupported)

	got, err := os.ReadFile(filepath.Join(dirPath, oldName))
	require.NoError(t, err)
	require.Equal(t, contents, got)
	_, err = os.Lstat(filepath.Join(dirPath, newName))
	require.ErrorIs(t, err, os.ErrNotExist)
}

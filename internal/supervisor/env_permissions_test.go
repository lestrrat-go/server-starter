//go:build !windows

package supervisor

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReloadEnvdirReportsUnreadableDirectory(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can read directories without permission bits")
	}

	path := filepath.Join(t.TempDir(), "unreadable")
	require.NoError(t, os.Mkdir(path, 0700))
	require.NoError(t, os.Chmod(path, 0))
	t.Cleanup(func() { require.NoError(t, os.Chmod(path, 0700)) })

	got, err := reloadEnv(path)
	require.Nil(t, got)
	require.ErrorIs(t, err, fs.ErrPermission)
	require.Contains(t, err.Error(), path)
}

func TestReloadEnvdirReportsUnreadableEntry(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can read files without permission bits")
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "SECRET")
	require.NoError(t, os.WriteFile(path, []byte("value"), 0600))
	require.NoError(t, os.Chmod(path, 0))

	got, err := reloadEnv(dir)
	require.Nil(t, got)
	require.ErrorIs(t, err, fs.ErrPermission)
	require.Contains(t, err.Error(), path)
}

//go:build !windows

package cli

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOpenLogFilePermissions(t *testing.T) {
	originalUmask := syscall.Umask(0)
	t.Cleanup(func() {
		syscall.Umask(originalUmask)
	})

	t.Run("new file is private", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "server.log")
		f, err := openLogFile(path)
		require.NoError(t, err)
		require.NoError(t, f.Close())

		info, err := os.Stat(path)
		require.NoError(t, err)
		require.Equal(t, os.FileMode(0600), info.Mode().Perm())
	})

	t.Run("existing file permissions are preserved", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "server.log")
		require.NoError(t, os.WriteFile(path, nil, 0644))

		f, err := openLogFile(path)
		require.NoError(t, err)
		require.NoError(t, f.Close())

		info, err := os.Stat(path)
		require.NoError(t, err)
		require.Equal(t, os.FileMode(0644), info.Mode().Perm())
	})
}

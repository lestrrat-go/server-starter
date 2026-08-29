package supervisor

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRemoveExistingUnixSocketRemovesStaleSocket(t *testing.T) {
	path := filepath.Join(t.TempDir(), "listener.sock")
	makeStaleSocket(t, path)

	require.NoError(t, removeExistingUnixSocket(path))
	_, err := os.Lstat(path)
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestRemoveExistingUnixSocketRejectsNonSocket(t *testing.T) {
	for _, test := range []struct {
		name string
		make func(string) error
	}{
		{name: "file", make: func(path string) error { return os.WriteFile(path, []byte("keep"), 0o600) }},
		{name: "directory", make: func(path string) error { return os.Mkdir(path, 0o700) }},
		{name: "symlink", make: func(path string) error { return os.Symlink("target", path) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "listener")
			require.NoError(t, test.make(path))

			err := removeExistingUnixSocket(path)
			require.ErrorContains(t, err, "is not a socket")
			_, statErr := os.Lstat(path)
			require.NoError(t, statErr)
		})
	}
}

func TestRemoveExistingUnixSocketPreservesReplacement(t *testing.T) {
	path := filepath.Join(t.TempDir(), "listener.sock")
	makeStaleSocket(t, path)

	var replacement net.Listener
	var err error
	move := func(oldpath, newpath string) error {
		require.NoError(t, os.Remove(oldpath))
		replacement, err = (&net.ListenConfig{}).Listen(context.Background(), unixNetwork, oldpath)
		if err != nil {
			return err
		}
		return os.Rename(oldpath, newpath)
	}
	err = removeSocketWithIdentity(path, mustLstat(t, path), move)
	require.ErrorContains(t, err, "changed during preparation")
	info, statErr := os.Lstat(path)
	require.NoError(t, statErr)
	require.NotZero(t, info.Mode()&os.ModeSocket)
	require.NoError(t, replacement.Close())
}

func makeStaleSocket(t *testing.T, path string) {
	t.Helper()
	listener, err := (&net.ListenConfig{}).Listen(context.Background(), unixNetwork, path)
	require.NoError(t, err)
	backup := path + ".stale"
	require.NoError(t, os.Rename(path, backup))
	require.NoError(t, listener.Close())
	require.NoError(t, os.Rename(backup, path))
}

func mustLstat(t *testing.T, path string) os.FileInfo {
	info, err := os.Lstat(path)
	require.NoError(t, err)
	return info
}

func TestRemoveExistingUnixSocketSkipsLinuxAbstractAddress(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux abstract addresses are Linux-only")
	}
	require.NoError(t, removeExistingUnixSocket("@server-starter-test"))
}

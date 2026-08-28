//go:build linux

package supervisor

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRemoveExistingUnixSocketAllowsAbstractAddress(t *testing.T) {
	dir := t.TempDir()
	oldwd, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() {
		require.NoError(t, os.Chdir(oldwd))
	})

	path := fmt.Sprintf("@server-starter-%d-%s", os.Getpid(), filepath.Base(dir))
	contents := []byte("keep me")
	require.NoError(t, os.WriteFile(path, contents, 0600))
	require.NoError(t, removeExistingUnixSocket(path))

	l, err := (&net.ListenConfig{}).Listen(context.Background(), "unix", path)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, l.Close())
	})

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, contents, got)
}

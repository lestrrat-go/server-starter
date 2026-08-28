//go:build !windows

package statefile_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/lestrrat-go/server-starter/v2/internal/statefile"
	"github.com/stretchr/testify/require"
)

func TestAcquireRejectsSymlinkWithoutChangingTarget(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	path := filepath.Join(dir, "server.pid")
	require.NoError(t, os.WriteFile(target, []byte("keep me"), 0600))
	require.NoError(t, os.Symlink(target, path))

	pidFile, err := statefile.Acquire(path)
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

	pidFile, err := statefile.Acquire(path)
	require.Error(t, err)
	require.Nil(t, pidFile)

	data, err := os.ReadFile(target)
	require.NoError(t, err)
	require.Equal(t, "keep me", string(data))
}

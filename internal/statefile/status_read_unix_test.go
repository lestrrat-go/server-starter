//go:build !windows

package statefile_test

import (
	"context"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/lestrrat-go/server-starter/v2/internal/statefile"
	"github.com/stretchr/testify/require"
)

func TestReadPIDRejectsSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	require.NoError(t, os.WriteFile(target, []byte("1\n"), 0600))
	path := filepath.Join(dir, "pid")
	require.NoError(t, os.Symlink(target, path))

	_, err := statefile.ReadPID(context.Background(), path)
	require.Error(t, err)
}

func TestReadStatusRejectsFIFOWithoutBlocking(t *testing.T) {
	path := filepath.Join(t.TempDir(), "status")
	require.NoError(t, syscall.Mkfifo(path, 0600))

	_, err := statefile.ReadStatus(context.Background(), path)
	require.ErrorContains(t, err, "not a regular file")
}

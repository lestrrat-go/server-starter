//go:build linux

package statefile

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLinuxFlockOwnerReturnsOpenError(t *testing.T) {
	f, err := os.Create(filepath.Join(t.TempDir(), "server.pid"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, f.Close()) })

	_, err = linuxFlockOwnerAt(f, filepath.Join(t.TempDir(), "missing"))
	require.Error(t, err)
}

func TestLinuxFlockOwnerReturnsScanError(t *testing.T) {
	f, err := os.Create(filepath.Join(t.TempDir(), "server.pid"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, f.Close()) })

	_, err = linuxFlockOwnerAt(f, t.TempDir())
	require.Error(t, err)
}

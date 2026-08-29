//go:build linux

package statefile

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLinuxFlockOwnerUnavailableIsUnknown(t *testing.T) {
	f, err := os.Create(filepath.Join(t.TempDir(), "server.pid"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, f.Close()) })

	ownerPID, err := linuxFlockOwnerAt(f, filepath.Join(t.TempDir(), "missing"))
	require.NoError(t, err)
	require.Zero(t, ownerPID)
}

func TestLinuxFlockOwnerUnreadableIsUnknown(t *testing.T) {
	f, err := os.Create(filepath.Join(t.TempDir(), "server.pid"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, f.Close()) })

	ownerPID, err := linuxFlockOwnerAt(f, t.TempDir())
	require.NoError(t, err)
	require.Zero(t, ownerPID)
}

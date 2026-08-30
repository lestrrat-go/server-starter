//go:build linux

package statefile

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOpenRunningPIDRejectsUnownedPIDFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "server.pid")
	require.NoError(t, os.WriteFile(path, []byte(strconv.Itoa(os.Getpid())+"\n"), 0600))

	running, err := OpenRunningPID(path)
	require.Error(t, err)
	require.Nil(t, running)
	require.ErrorContains(t, err, "lock owner could not be verified")
}

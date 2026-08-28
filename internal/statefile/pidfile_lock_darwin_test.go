//go:build darwin

package statefile_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/lestrrat-go/server-starter/v2/internal/statefile"
	"github.com/stretchr/testify/require"
)

func TestAcquirePIDFileSucceedsOnDarwin(t *testing.T) {
	path := filepath.Join(t.TempDir(), "server.pid")
	pidFile, err := statefile.Acquire(path)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, pidFile.Close())
	})

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, fmt.Sprintf("%d\n", os.Getpid()), string(data))
}

func TestAcquirePIDFileRejectsOtherDarwinProcess(t *testing.T) {
	path := filepath.Join(t.TempDir(), "server.pid")
	ownerPID := startPIDLockHelper(t, path)

	pidFile, err := statefile.Acquire(path)
	require.ErrorIs(t, err, statefile.ErrPIDFileLocked)
	require.ErrorContains(t, err, strconv.Itoa(ownerPID))
	require.Nil(t, pidFile)
}

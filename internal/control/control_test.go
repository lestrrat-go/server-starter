package control

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/lestrrat-go/server-starter/v2/internal/statefile"
	"github.com/stretchr/testify/require"
)

func TestGenerationTransitions(t *testing.T) {
	status := map[int]int{1: 100, 2: 200}
	require.True(t, generationAdvanced(map[int]int{1: 100}, status), "status did not advance")
	require.True(t, oldWorkersGone(map[int]int{1: 100}, map[int]int{2: 200}), "old worker was reported as alive")
}

func TestProcessStoppedPollingStates(t *testing.T) {
	t.Run("missing pid file means stopped", func(t *testing.T) {
		stopped, err := processStopped(filepath.Join(t.TempDir(), "missing.pid"), statefile.TryLock)
		require.NoError(t, err)
		require.True(t, stopped)
	})

	t.Run("lock contention means keep polling", func(t *testing.T) {
		pidPath := filepath.Join(t.TempDir(), "server.pid")
		require.NoError(t, os.WriteFile(pidPath, []byte("1\n"), 0600))

		stopped, err := processStopped(pidPath, func(*os.File) error {
			return syscall.EWOULDBLOCK
		})
		require.NoError(t, err)
		require.False(t, stopped)
	})

	t.Run("persistent lock error fails polling", func(t *testing.T) {
		pidPath := filepath.Join(t.TempDir(), "server.pid")
		require.NoError(t, os.WriteFile(pidPath, []byte("1\n"), 0600))
		lockErr := errors.New("lock failed")

		stopped, err := processStopped(pidPath, func(*os.File) error {
			return lockErr
		})
		require.False(t, stopped)
		require.ErrorIs(t, err, lockErr)
		require.ErrorContains(t, err, "failed to check pid file")
	})
}

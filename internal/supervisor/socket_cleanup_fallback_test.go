//go:build !linux

package supervisor

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRunFallbackRemovesUnixSocketOnClose(t *testing.T) {
	path := filepath.Join(t.TempDir(), "listener.sock")
	command, args := testWorkerCommand(t)
	starter, err := NewStarter(&config{
		command:   command,
		args:      args,
		paths:     []string{path},
		sigonterm: signalNameKill,
	})
	require.NoError(t, err)

	for range 2 {
		ctx, cancel := context.WithCancel(context.Background())
		ctrl, runErr := starter.Run(ctx)
		require.NoError(t, runErr)
		cancel()
		require.ErrorIs(t, ctrl.Wait(), ErrServerClosed)
		_, statErr := os.Lstat(path)
		require.ErrorIs(t, statErr, os.ErrNotExist)
	}
}

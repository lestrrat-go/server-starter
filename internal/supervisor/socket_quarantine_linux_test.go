package supervisor

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRunStartsUnixListenerInWriteSearchOnlyParent(t *testing.T) {
	parent := t.TempDir()
	t.Cleanup(func() { require.NoError(t, os.Chmod(parent, 0o700)) })
	require.NoError(t, os.Chmod(parent, 0o300))
	path := filepath.Join(parent, "listener.sock")
	command, args := testWorkerCommand(t)
	starter, err := NewStarter(&config{
		command:   command,
		args:      args,
		paths:     []string{path},
		sigonterm: signalNameKill,
	})
	require.NoError(t, err)

	// testing.T.Context requires Go 1.24, but this module supports Go 1.23.
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	ctrl, err := starter.Run(ctx)
	require.NoError(t, err)
	info, err := os.Lstat(path)
	require.NoError(t, err)
	require.NotZero(t, info.Mode()&os.ModeSocket)

	cancel()
	require.ErrorIs(t, ctrl.Wait(), ErrServerClosed)
}

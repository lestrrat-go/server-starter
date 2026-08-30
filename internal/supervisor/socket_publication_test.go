//go:build linux || darwin

package supervisor

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRunRejectsUnixSocketPublicationCollision(t *testing.T) {
	path := filepath.Join(t.TempDir(), "server.sock")
	command, args := testWorkerCommand(t)
	starter, err := NewStarter(&config{
		command: command,
		args:    args,
		paths:   []string{path},
	})
	require.NoError(t, err)

	replacement := []byte("replacement")
	ctrl, err := starter.run(context.Background(), false, socketPublicationHooks{
		beforePublish: func(publicPath string) error {
			return os.WriteFile(publicPath, replacement, 0o600)
		},
	})
	require.Nil(t, ctrl)
	require.ErrorContains(t, err, "publish unix socket")

	contents, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, replacement, contents)
}

func TestRunCapturesUnixSocketIdentityBeforePublication(t *testing.T) {
	path := filepath.Join(t.TempDir(), "server.sock")
	ownedPath := path + ".owned"
	command, args := testWorkerCommand(t)
	starter, err := NewStarter(&config{
		command:   command,
		args:      args,
		paths:     []string{path},
		sigonterm: signalNameKill,
	})
	require.NoError(t, err)

	replacement := []byte("replacement")
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	ctrl, err := starter.run(ctx, false, socketPublicationHooks{
		afterPublish: func(publicPath string) error {
			if err := os.Rename(publicPath, ownedPath); err != nil {
				return err
			}
			return os.WriteFile(publicPath, replacement, 0o600)
		},
	})
	require.NoError(t, err)

	cancel()
	require.ErrorIs(t, ctrl.Wait(), ErrServerClosed)
	contents, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, replacement, contents)
	info, err := os.Lstat(ownedPath)
	require.NoError(t, err)
	require.NotZero(t, info.Mode()&os.ModeSocket)
}

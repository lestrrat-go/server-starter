//go:build linux || darwin

package supervisor

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
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

func TestListenFilesystemUnixSocketPublishesFromRetainedPrivateDirectory(t *testing.T) {
	parent := t.TempDir()
	publicPath := filepath.Join(parent, "listener.sock")
	var replacementListener net.Listener
	var replacementPath string
	var lc net.ListenConfig

	listener, identity, err := listenFilesystemUnixSocket(
		context.Background(),
		publicPath,
		socketPublicationHooks{beforePublish: func(string) error {
			entries, readErr := os.ReadDir(parent)
			if readErr != nil {
				return readErr
			}
			if len(entries) != 1 {
				return fmt.Errorf("private directory count is %d, want 1", len(entries))
			}
			privateDir := filepath.Join(parent, entries[0].Name())
			if renameErr := os.Rename(privateDir, privateDir+".moved"); renameErr != nil {
				return renameErr
			}
			if mkdirErr := os.Mkdir(privateDir, 0o700); mkdirErr != nil {
				return mkdirErr
			}
			replacementPath = filepath.Join(privateDir, "s")
			replacementListener, readErr = lc.Listen(context.Background(), unixNetwork, replacementPath)
			return readErr
		}},
	)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, listener.Close())
		require.NoError(t, replacementListener.Close())
	})

	publishedIdentity, err := socketIdentityForPath(publicPath)
	require.NoError(t, err)
	require.Equal(t, identity, publishedIdentity)
	replacementIdentity, err := socketIdentityForPath(replacementPath)
	require.NoError(t, err)
	require.NotEqual(t, identity, replacementIdentity)
}

func TestListenFilesystemUnixSocketAcceptsNearLimitPublicPath(t *testing.T) {
	root := t.TempDir()
	const publicPathLength = 100
	parentNameLength := publicPathLength - len(root) - len("//s")
	require.Positive(t, parentNameLength)
	parent := filepath.Join(root, strings.Repeat("p", parentNameLength))
	require.NoError(t, os.Mkdir(parent, 0o700))
	publicPath := filepath.Join(parent, "s")
	require.Len(t, publicPath, publicPathLength)

	var lc net.ListenConfig
	directListener, err := lc.Listen(context.Background(), unixNetwork, publicPath)
	require.NoError(t, err)
	require.NoError(t, directListener.Close())

	listener, _, err := listenFilesystemUnixSocket(context.Background(), publicPath, socketPublicationHooks{})
	require.NoError(t, err)
	require.NoError(t, listener.Close())
}

func TestListenFilesystemUnixSocketRejectsChangedPrivateSocket(t *testing.T) {
	parent := t.TempDir()
	publicPath := filepath.Join(parent, "listener.sock")
	var replacementListener net.Listener
	var lc net.ListenConfig

	listener, _, err := listenFilesystemUnixSocket(
		context.Background(),
		publicPath,
		socketPublicationHooks{beforePublish: func(string) error {
			entries, readErr := os.ReadDir(parent)
			if readErr != nil {
				return readErr
			}
			if len(entries) != 1 {
				return fmt.Errorf("private directory count is %d, want 1", len(entries))
			}
			privatePath := filepath.Join(parent, entries[0].Name(), "s")
			if removeErr := os.Remove(privatePath); removeErr != nil {
				return removeErr
			}
			replacementListener, readErr = lc.Listen(context.Background(), unixNetwork, privatePath)
			return readErr
		}},
	)
	require.Nil(t, listener)
	require.ErrorIs(t, err, errSocketPublicationSourceChanged)
	require.NoFileExists(t, publicPath)
	require.NotNil(t, replacementListener)
	require.NoError(t, replacementListener.Close())
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

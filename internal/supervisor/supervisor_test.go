package supervisor

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRunRejectsSparseDescriptorLayout(t *testing.T) {
	command, err := os.Executable()
	require.NoError(t, err)

	sd, err := NewStarter(&config{
		command: command,
		ports:   []string{fmt.Sprintf("0=%d", maxSparseListenerFDSlots+4)},
	})
	require.NoError(t, err)

	ctrl, err := sd.Run(context.Background())
	require.Nil(t, ctrl)
	require.ErrorContains(t, err, fmt.Sprintf("maximum is %d", maxSparseListenerFDSlots))
}

const testShellPath = "/bin/sh"

func TestTeardownRemovesOwnedUnixSocket(t *testing.T) {
	requireSafeSocketQuarantine(t)
	path := filepath.Join(t.TempDir(), "server.sock")
	l, err := (&net.ListenConfig{}).Listen(context.Background(), unixNetwork, path)
	if err != nil {
		t.Fatal(err)
	}
	l.(*net.UnixListener).SetUnlinkOnClose(false)
	identity, err := socketIdentityForPath(path)
	require.NoError(t, err)
	cleanup, err := socketCleanupStateForPath(path, identity)
	require.NoError(t, err)
	rs := &runState{
		cfg: &Starter{paths: []string{path}},
		listeners: []listener{{
			listener:      l,
			network:       unixNetwork,
			path:          path,
			socketCleanup: cleanup,
		}},
	}
	rs.teardown()
	_, err = os.Lstat(path)
	require.ErrorIs(t, err, os.ErrNotExist)
}

type replacementOnCloseListener struct {
	net.Listener
	path string
}

func (l *replacementOnCloseListener) Close() error {
	if err := os.Rename(l.path, l.path+".owned"); err != nil {
		return err
	}
	if err := l.Listener.Close(); err != nil {
		return err
	}
	return os.WriteFile(l.path, []byte("keep"), 0o600)
}

func TestTeardownPreservesUnixSocketPathReplacement(t *testing.T) {
	requireSafeSocketQuarantine(t)
	path := filepath.Join(t.TempDir(), "server.sock")
	bound, err := (&net.ListenConfig{}).Listen(context.Background(), unixNetwork, path)
	require.NoError(t, err)
	bound.(*net.UnixListener).SetUnlinkOnClose(false)
	identity, err := socketIdentityForPath(path)
	require.NoError(t, err)
	cleanup, err := socketCleanupStateForPath(path, identity)
	require.NoError(t, err)

	rs := &runState{
		cfg: &Starter{paths: []string{path}},
		listeners: []listener{{
			listener:      &replacementOnCloseListener{Listener: bound, path: path},
			network:       unixNetwork,
			path:          path,
			socketCleanup: cleanup,
		}},
	}
	rs.teardown()

	contents, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, []byte("keep"), contents)
	info, err := os.Lstat(path + ".owned")
	require.NoError(t, err)
	require.NotZero(t, info.Mode()&os.ModeSocket)
}

func TestTeardownUsesRetainedUnixSocketParent(t *testing.T) {
	requireSafeSocketQuarantine(t)
	realParent := t.TempDir()
	aliasParent := filepath.Join(t.TempDir(), "alias")
	require.NoError(t, os.Symlink(realParent, aliasParent))
	path := filepath.Join(aliasParent, "listener.sock")
	command, args := testWorkerCommand(t)
	starter, err := NewStarter(&config{
		command:   command,
		args:      args,
		paths:     []string{path},
		sigonterm: signalNameKill,
	})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	ctrl, err := starter.Run(ctx)
	require.NoError(t, err)
	require.NoError(t, os.Remove(aliasParent))
	replacementParent := t.TempDir()
	require.NoError(t, os.Symlink(replacementParent, aliasParent))
	cancel()
	require.ErrorIs(t, ctrl.Wait(), ErrServerClosed)

	_, err = os.Lstat(filepath.Join(realParent, "listener.sock"))
	require.ErrorIs(t, err, os.ErrNotExist)
	_, err = os.Lstat(filepath.Join(replacementParent, "listener.sock"))
	require.ErrorIs(t, err, os.ErrNotExist)
}

// TestRunErrServerClosed proves that cancelling the context passed to Run
// is reported as a clean shutdown: ctrl.Wait() must return an error that
// satisfies errors.Is(err, ErrServerClosed), never nil and never some other
// error, so callers can treat context cancellation as success.
func TestRunErrServerClosed(t *testing.T) {
	command, args := testWorkerCommand(t)
	sd, err := NewStarter(&config{
		command:   command,
		args:      args,
		ports:     testWorkerPorts(),
		sigonterm: signalNameKill,
	})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ctrl, err := sd.Run(ctx)
	require.NoError(t, err)

	cancel()

	errCh := make(chan error, 1)
	go func() { errCh <- ctrl.Wait() }()

	select {
	case err := <-errCh:
		require.ErrorIs(t, err, ErrServerClosed)
	case <-time.After(20 * time.Second):
		t.Fatal("timed out waiting for ctrl.Wait() to return")
	}
}

func TestRunRejectsInvalidListenerMetadataBeforeBinding(t *testing.T) {
	command, err := os.Executable()
	require.NoError(t, err)

	for _, test := range []struct {
		name string
		spec string
	}{
		{name: "TCP NUL", spec: "127.0.0.1\x00bad:0"},
		{name: "UDP NUL", spec: "u127.0.0.1\x00bad:0"},
	} {
		t.Run(test.name, func(t *testing.T) {
			sd, err := NewStarter(&config{command: command, ports: []string{test.spec}})
			require.NoError(t, err)

			ctrl, err := sd.Run(context.Background())
			require.Nil(t, ctrl)
			require.ErrorContains(t, err, "NUL")
		})
	}

	for _, test := range []struct {
		name    string
		path    string
		wantErr string
	}{
		{name: "Unix NUL", path: "listener\x00ignored.sock", wantErr: "NUL"},
		{name: "Unix delimiter", path: "listener;ignored.sock", wantErr: "must not contain"},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), test.path)
			sd, err := NewStarter(&config{command: command, paths: []string{path}})
			require.NoError(t, err)

			ctrl, err := sd.Run(context.Background())
			require.Nil(t, ctrl)
			require.ErrorContains(t, err, test.wantErr)
		})
	}
}

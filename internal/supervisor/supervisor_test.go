package supervisor

import (
	"context"
	"fmt"
	"io"
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

func TestTeardownRemovesUnixSocket(t *testing.T) {
	path := filepath.Join(t.TempDir(), "server.sock")
	l, err := (&net.ListenConfig{}).Listen(context.Background(), unixNetwork, path)
	if err != nil {
		t.Fatal(err)
	}
	rs := &runState{cfg: &Starter{}, listeners: []listener{{listener: l, network: unixNetwork, path: path}}}
	rs.teardown()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("unix socket path remains, stat error = %v", err)
	}
}

func TestRemoveExistingUnixSocketRejectsNonSocketEntries(t *testing.T) {
	t.Run("regular file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "server.sock")
		contents := []byte("keep me")
		require.NoError(t, os.WriteFile(path, contents, 0600))

		err := removeExistingUnixSocket(path)
		require.ErrorContains(t, err, "is not a socket")
		got, readErr := os.ReadFile(path)
		require.NoError(t, readErr)
		require.Equal(t, contents, got)
	})

	t.Run("directory", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "server.sock")
		require.NoError(t, os.Mkdir(path, 0700))

		renameCalled := false
		err := removeExistingUnixSocketWithRename(path, func(*os.File, string, string) error {
			renameCalled = true
			return nil
		})
		require.ErrorContains(t, err, "is not a socket")
		require.False(t, renameCalled)
		info, statErr := os.Stat(path)
		require.NoError(t, statErr)
		require.True(t, info.IsDir())
	})

	t.Run("symbolic link", func(t *testing.T) {
		dir := t.TempDir()
		target := filepath.Join(dir, "target")
		path := filepath.Join(dir, "server.sock")
		contents := []byte("keep me")
		require.NoError(t, os.WriteFile(target, contents, 0600))
		if err := os.Symlink(target, path); err != nil {
			t.Skipf("symbolic links are unavailable: %s", err)
		}

		err := removeExistingUnixSocket(path)
		require.ErrorContains(t, err, "is not a socket")
		info, statErr := os.Lstat(path)
		require.NoError(t, statErr)
		require.NotZero(t, info.Mode()&os.ModeSymlink)
		got, readErr := os.ReadFile(target)
		require.NoError(t, readErr)
		require.Equal(t, contents, got)
	})
}

func TestRunRejectsExistingNonSocketPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "server.sock")
	contents := []byte("keep me")
	require.NoError(t, os.WriteFile(path, contents, 0600))
	starter := &Starter{paths: []string{path}, stderr: io.Discard}

	ctrl, err := starter.Run(context.Background())
	require.Nil(t, ctrl)
	require.ErrorContains(t, err, "is not a socket")
	got, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	require.Equal(t, contents, got)
}

func TestRemoveExistingUnixSocketRemovesSocket(t *testing.T) {
	path := filepath.Join(t.TempDir(), "server.sock")
	addr, err := net.ResolveUnixAddr("unix", path)
	require.NoError(t, err)
	l, err := net.ListenUnix("unix", addr)
	require.NoError(t, err)
	l.SetUnlinkOnClose(false)
	require.NoError(t, l.Close())

	require.NoError(t, removeExistingUnixSocket(path))
	_, err = os.Lstat(path)
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestRemoveExistingUnixSocketPreservesReplacement(t *testing.T) {
	t.Run("replacement before move", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "server.sock")
		addr, err := net.ResolveUnixAddr("unix", path)
		require.NoError(t, err)
		l, err := net.ListenUnix("unix", addr)
		require.NoError(t, err)
		l.SetUnlinkOnClose(false)
		require.NoError(t, l.Close())

		renameReached := make(chan struct{})
		continueRename := make(chan struct{}, 1)
		result := make(chan error, 1)
		t.Cleanup(func() {
			select {
			case continueRename <- struct{}{}:
			default:
			}
		})

		go func() {
			result <- removeExistingUnixSocketWithRename(path, func(dir *os.File, oldName, newName string) error {
				close(renameReached)
				<-continueRename
				return renameNoReplaceAt(dir, oldName, newName)
			})
		}()

		<-renameReached
		require.NoError(t, os.Remove(path))
		contents := []byte("replacement")
		require.NoError(t, os.WriteFile(path, contents, 0600))
		continueRename <- struct{}{}

		require.ErrorContains(t, <-result, "is not a socket")
		got, err := os.ReadFile(path)
		require.NoError(t, err)
		require.Equal(t, contents, got)
		entries, err := os.ReadDir(dir)
		require.NoError(t, err)
		require.Len(t, entries, 1)
	})

	t.Run("replacement after move", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "server.sock")
		addr, err := net.ResolveUnixAddr("unix", path)
		require.NoError(t, err)
		l, err := net.ListenUnix("unix", addr)
		require.NoError(t, err)
		l.SetUnlinkOnClose(false)
		require.NoError(t, l.Close())

		renameFinished := make(chan struct{})
		continueRemoval := make(chan struct{}, 1)
		result := make(chan error, 1)
		t.Cleanup(func() {
			select {
			case continueRemoval <- struct{}{}:
			default:
			}
		})

		go func() {
			result <- removeExistingUnixSocketWithRename(path, func(dir *os.File, oldName, newName string) error {
				if err := renameNoReplaceAt(dir, oldName, newName); err != nil {
					return err
				}
				close(renameFinished)
				<-continueRemoval
				return nil
			})
		}()

		<-renameFinished
		contents := []byte("replacement")
		require.NoError(t, os.WriteFile(path, contents, 0600))
		continueRemoval <- struct{}{}

		require.NoError(t, <-result)
		got, err := os.ReadFile(path)
		require.NoError(t, err)
		require.Equal(t, contents, got)
		entries, err := os.ReadDir(dir)
		require.NoError(t, err)
		require.Len(t, entries, 1)
	})
}

func TestRemoveExistingUnixSocketAnchorsParentDirectory(t *testing.T) {
	root := t.TempDir()
	parentPath := filepath.Join(root, "parent")
	movedParentPath := filepath.Join(root, "moved-parent")
	require.NoError(t, os.Mkdir(parentPath, 0700))

	path := filepath.Join(parentPath, "server.sock")
	addr, err := net.ResolveUnixAddr("unix", path)
	require.NoError(t, err)
	l, err := net.ListenUnix("unix", addr)
	require.NoError(t, err)
	l.SetUnlinkOnClose(false)
	require.NoError(t, l.Close())

	contents := []byte("replacement")
	err = removeExistingUnixSocketWithRename(path, func(dir *os.File, oldName, newName string) error {
		require.NoError(t, os.Rename(parentPath, movedParentPath))
		require.NoError(t, os.Mkdir(parentPath, 0700))
		require.NoError(t, os.WriteFile(path, contents, 0600))
		return renameNoReplaceAt(dir, oldName, newName)
	})
	require.NoError(t, err)

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, contents, got)
	entries, err := os.ReadDir(movedParentPath)
	require.NoError(t, err)
	require.Empty(t, entries)
}

func TestRemoveExistingUnixSocketAllowsMissingPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "server.sock")
	require.NoError(t, removeExistingUnixSocket(path))
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
		sigonterm: "KILL",
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

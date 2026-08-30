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

func TestTeardownPreservesUnixSocketPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "server.sock")
	l, err := (&net.ListenConfig{}).Listen(context.Background(), unixNetwork, path)
	if err != nil {
		t.Fatal(err)
	}
	l.(*net.UnixListener).SetUnlinkOnClose(false)
	rs := &runState{cfg: &Starter{}, listeners: []listener{{listener: l, network: unixNetwork, path: path}}}
	rs.teardown()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("unix socket path was removed, stat error = %v", err)
	}
}

func TestValidateUnixSocketPathAvailableRejectsExistingEntries(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")

	tests := []struct {
		name string
		make func(t *testing.T) string
	}{
		{name: "regular file", make: func(t *testing.T) string {
			path := filepath.Join(dir, "regular")
			require.NoError(t, os.WriteFile(path, []byte("keep"), 0o600))
			return path
		}},
		{name: "directory", make: func(t *testing.T) string {
			path := filepath.Join(dir, "directory")
			require.NoError(t, os.Mkdir(path, 0o700))
			return path
		}},
		{name: "symlink", make: func(t *testing.T) string {
			path := filepath.Join(dir, "symlink")
			require.NoError(t, os.Symlink(target, path))
			return path
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := test.make(t)
			require.ErrorContains(t, validateUnixSocketPathAvailable(path), "already exists")
			_, err := os.Lstat(path)
			require.NoError(t, err)
		})
	}
}

func TestRunPreservesExistingUnixEntries(t *testing.T) {
	command, err := os.Executable()
	require.NoError(t, err)
	dir := t.TempDir()
	target := filepath.Join(dir, "target")

	entries := []struct {
		name string
		make func(t *testing.T) string
	}{
		{name: "regular file", make: func(t *testing.T) string {
			path := filepath.Join(dir, "regular")
			require.NoError(t, os.WriteFile(path, []byte("keep"), 0o600))
			return path
		}},
		{name: "directory", make: func(t *testing.T) string {
			path := filepath.Join(dir, "directory")
			require.NoError(t, os.Mkdir(path, 0o700))
			return path
		}},
		{name: "symlink", make: func(t *testing.T) string {
			path := filepath.Join(dir, "symlink")
			require.NoError(t, os.Symlink(target, path))
			return path
		}},
		{name: "stale socket", make: func(t *testing.T) string {
			path := filepath.Join(dir, "stale.sock")
			listener, err := (&net.ListenConfig{}).Listen(context.Background(), unixNetwork, path)
			require.NoError(t, err)
			listener.(*net.UnixListener).SetUnlinkOnClose(false)
			require.NoError(t, listener.Close())
			return path
		}},
	}

	for _, entry := range entries {
		t.Run(entry.name, func(t *testing.T) {
			path := entry.make(t)
			sd, err := NewStarter(&config{command: command, paths: []string{path}})
			require.NoError(t, err)

			ctrl, err := sd.Run(context.Background())
			require.Nil(t, ctrl)
			require.ErrorContains(t, err, "already exists")
			_, err = os.Lstat(path)
			require.NoError(t, err)
		})
	}
}

func TestValidateUnixSocketPathAvailableAllowsNonFilesystemAddresses(t *testing.T) {
	for _, path := range []string{"", "@server-starter-test", "\x00server-starter-test"} {
		require.NoError(t, validateUnixSocketPathAvailable(path))
	}
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
		sigonterm: killSignalName,
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

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
	tests := map[string]func(*testing.T, string){
		"regular file": func(t *testing.T, path string) {
			require.NoError(t, os.WriteFile(path, []byte("keep me"), 0600))
		},
		"directory": func(t *testing.T, path string) {
			require.NoError(t, os.Mkdir(path, 0700))
		},
		"symbolic link": func(t *testing.T, path string) {
			target := filepath.Join(t.TempDir(), "target")
			require.NoError(t, os.WriteFile(target, []byte("keep me"), 0600))
			if err := os.Symlink(target, path); err != nil {
				t.Skipf("symbolic links are unavailable: %s", err)
			}
		},
	}

	for name, create := range tests {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "server.sock")
			create(t, path)

			err := removeExistingUnixSocket(path)
			require.ErrorContains(t, err, "is not a socket")
			_, statErr := os.Lstat(path)
			require.NoError(t, statErr)
		})
	}
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
	addr, err := net.ResolveUnixAddr(unixNetwork, path)
	require.NoError(t, err)
	l, err := net.ListenUnix(unixNetwork, addr)
	require.NoError(t, err)
	l.SetUnlinkOnClose(false)
	require.NoError(t, l.Close())

	require.NoError(t, removeExistingUnixSocket(path))
	_, err = os.Lstat(path)
	require.ErrorIs(t, err, os.ErrNotExist)
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

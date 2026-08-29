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

func TestTeardownRemovesUnixSocket(t *testing.T) {
	path := filepath.Join(t.TempDir(), "server.sock")
	l, err := (&net.ListenConfig{}).Listen(context.Background(), "unix", path)
	if err != nil {
		t.Fatal(err)
	}
	rs := &runState{cfg: &Starter{}, listeners: []listener{{listener: l, network: "unix", path: path}}}
	rs.teardown()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("unix socket path remains, stat error = %v", err)
	}
}

// TestRunErrServerClosed proves that cancelling the context passed to Run
// is reported as a clean shutdown: ctrl.Wait() must return an error that
// satisfies errors.Is(err, ErrServerClosed), never nil and never some other
// error, so callers can treat context cancellation as success.
func TestRunErrServerClosed(t *testing.T) {
	sd, err := NewStarter(&config{
		command: "/bin/sh",
		args:    []string{"-c", "exec sleep 30"},
		ports:   []string{"0"},
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

func TestRunReportsListenerMetadataErrorsAtLifecycleStage(t *testing.T) {
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
			require.Error(t, err)
			var opErr *net.OpError
			require.ErrorAs(t, err, &opErr)
			require.Nil(t, ctrl)
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
			require.NoError(t, err)
			require.NotNil(t, ctrl)

			select {
			case <-ctrl.Done():
			case <-time.After(5 * time.Second):
				t.Fatal("timed out waiting for listener metadata formatting failure")
			}
			require.ErrorContains(t, ctrl.Err(), "failed to format listeners for worker")
			require.ErrorContains(t, ctrl.Err(), test.wantErr)
		})
	}
}

package supervisor

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestTeardownRemovesUnixSocket(t *testing.T) {
	path := filepath.Join(t.TempDir(), "server.sock")
	l, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	rs := &runState{cfg: &Starter{}, listeners: []listener{{listener: l, spec: path}}}
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

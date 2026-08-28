package supervisor

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestTerminalWorkerStartError(t *testing.T) {
	tests := map[string]struct {
		err  error
		want bool
	}{
		"missing executable or directory": {err: syscall.ENOENT, want: true},
		"permission denied":               {err: syscall.EACCES, want: true},
		"invalid launch argument":         {err: syscall.EINVAL, want: true},
		"temporary resource exhaustion":   {err: syscall.EAGAIN, want: false},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, test.want, terminalWorkerStartError(test.err))
		})
	}
}

func TestWorkerStartRetryDelayHasFloor(t *testing.T) {
	require.Equal(t, minimumWorkerStartRetryDelay, workerStartRetryDelay(0))
	require.Equal(t, 3*time.Second, workerStartRetryDelay(3*time.Second))
}

func TestWaitForWorkerStartRetryHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	started := time.Now()
	require.False(t, waitForWorkerStartRetry(ctx, time.Hour))
	require.Less(t, time.Since(started), time.Second)
}

func TestMissingWorkingDirectoryStopsWorkerStartRetries(t *testing.T) {
	command, err := os.Executable()
	require.NoError(t, err)

	var stderr bytes.Buffer
	sd, err := NewStarter(&config{
		command:  command,
		dir:      filepath.Join(t.TempDir(), "missing"),
		interval: 60,
		stderr:   &stderr,
	})
	require.NoError(t, err)

	ctrl, err := sd.Run(context.Background())
	require.NoError(t, err)

	errCh := make(chan error, 1)
	go func() { errCh <- ctrl.Wait() }()

	select {
	case err := <-errCh:
		require.ErrorIs(t, err, os.ErrNotExist)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the terminal worker start error")
	}

	require.Equal(t, 1, strings.Count(stderr.String(), "failed to exec"))
}

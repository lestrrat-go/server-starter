package supervisor

import (
	"bytes"
	"context"
	"os"
	"os/exec"
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
		"missing executable or directory":   {err: syscall.ENOENT, want: true},
		"permission denied":                 {err: syscall.EACCES, want: true},
		"invalid launch argument":           {err: syscall.EINVAL, want: true},
		"invalid executable format":         {err: syscall.ENOEXEC, want: true},
		"path component is not directory":   {err: syscall.ENOTDIR, want: true},
		"symbolic link loop":                {err: syscall.ELOOP, want: true},
		"path name too long":                {err: syscall.ENAMETOOLONG, want: true},
		"argument list too long":            {err: syscall.E2BIG, want: true},
		"executable absent from PATH":       {err: exec.ErrNotFound, want: true},
		"executable found relative to PATH": {err: exec.ErrDot, want: true},
		"temporary resource exhaustion":     {err: syscall.EAGAIN, want: false},
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

func requireSingleTerminalStartAttempt(t *testing.T, cfg config, afterNew func(), want error) {
	t.Helper()

	var stderr bytes.Buffer
	cfg.stderr = &stderr
	sd, err := NewStarter(&cfg)
	require.NoError(t, err)
	if afterNew != nil {
		afterNew()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	ctrl, err := sd.Run(ctx)
	require.NoError(t, err)

	err = ctrl.Wait()
	require.ErrorIs(t, err, want, "stderr:\n%s", stderr.String())
	require.Equal(t, 1, strings.Count(stderr.String(), "failed to exec"), stderr.String())
}

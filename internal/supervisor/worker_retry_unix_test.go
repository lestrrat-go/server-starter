//go:build unix

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

func TestDeterministicLaunchErrorsStopWorkerStartRetries(t *testing.T) {
	executable, err := os.Executable()
	require.NoError(t, err)

	tests := map[string]struct {
		setup func(*testing.T) (config, func())
		want  error
	}{
		"invalid executable format": {
			setup: func(t *testing.T) (config, func()) {
				path := filepath.Join(t.TempDir(), "worker")
				require.NoError(t, os.WriteFile(path, []byte("not an executable\n"), 0o700))
				return config{command: path}, nil
			},
			want: syscall.ENOEXEC,
		},
		"path component is not directory": {
			setup: func(t *testing.T) (config, func()) {
				path := filepath.Join(t.TempDir(), "file")
				require.NoError(t, os.WriteFile(path, nil, 0o600))
				return config{command: executable, dir: filepath.Join(path, "child")}, nil
			},
			want: syscall.ENOTDIR,
		},
		"symbolic link loop": {
			setup: func(t *testing.T) (config, func()) {
				dir := t.TempDir()
				first := filepath.Join(dir, "first")
				second := filepath.Join(dir, "second")
				require.NoError(t, os.Symlink(second, first))
				require.NoError(t, os.Symlink(first, second))
				return config{command: executable, dir: first}, nil
			},
			want: syscall.ELOOP,
		},
		"path name too long": {
			setup: func(t *testing.T) (config, func()) {
				dir := filepath.Join(t.TempDir(), strings.Repeat("x", 1024))
				return config{command: executable, dir: dir}, nil
			},
			want: syscall.ENAMETOOLONG,
		},
		"argument list too long": {
			setup: func(t *testing.T) (config, func()) {
				return config{command: executable, args: []string{strings.Repeat("x", 2<<20)}}, nil
			},
			want: syscall.E2BIG,
		},
		"executable absent from PATH": {
			setup: func(t *testing.T) (config, func()) {
				dir := t.TempDir()
				path := filepath.Join(dir, "worker")
				require.NoError(t, os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o700))
				t.Setenv("PATH", dir)
				return config{command: "worker"}, func() {
					require.NoError(t, os.Remove(path))
				}
			},
			want: exec.ErrNotFound,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			cfg, afterNew := test.setup(t)
			requireSingleTerminalStartAttempt(t, cfg, afterNew, test.want)
		})
	}
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

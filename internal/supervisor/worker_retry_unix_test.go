//go:build unix

package supervisor

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

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
				return config{command: testWorkerCommandName}, func() {
					require.NoError(t, os.Remove(path))
				}
			},
			want: exec.ErrNotFound,
		},
		"executable found relative to PATH": {
			setup: func(t *testing.T) (config, func()) {
				preferredDir := t.TempDir()
				fallbackDir := t.TempDir()
				preferredPath := filepath.Join(preferredDir, "worker")
				fallbackPath := filepath.Join(fallbackDir, "worker")
				worker := []byte("#!/bin/sh\nexit 0\n")
				require.NoError(t, os.WriteFile(preferredPath, worker, 0o700))
				require.NoError(t, os.WriteFile(fallbackPath, worker, 0o700))

				previousDir, err := os.Getwd()
				require.NoError(t, err)
				require.NoError(t, os.Chdir(fallbackDir))
				t.Cleanup(func() {
					require.NoError(t, os.Chdir(previousDir))
				})

				t.Setenv("PATH", preferredDir+string(os.PathListSeparator)+".")
				return config{command: testWorkerCommandName}, func() {
					require.NoError(t, os.Remove(preferredPath))
				}
			},
			want: exec.ErrDot,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			cfg, afterNew := test.setup(t)
			requireSingleTerminalStartAttempt(t, cfg, afterNew, test.want)
		})
	}
}

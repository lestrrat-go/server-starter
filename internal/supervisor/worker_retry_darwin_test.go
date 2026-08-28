//go:build darwin

package supervisor

import (
	"debug/macho"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTerminalWorkerStartErrorRecognizesDarwinLaunchErrors(t *testing.T) {
	tests := map[string]error{
		"bad executable":         syscall.EBADEXEC,
		"unsupported CPU type":   syscall.EBADARCH,
		"shared library version": syscall.ESHLIBVERS,
		"malformed Mach-O":       syscall.EBADMACHO,
	}

	for name, startErr := range tests {
		t.Run(name, func(t *testing.T) {
			err := &os.PathError{
				Op:   "fork/exec",
				Path: "worker",
				Err:  startErr,
			}

			require.True(t, terminalWorkerStartError("worker", "", err))
		})
	}
}

func TestUnsupportedCPUTypeStopsWorkerStartRetries(t *testing.T) {
	executable, err := os.Executable()
	require.NoError(t, err)

	file, err := macho.Open(executable)
	require.NoError(t, err)
	byteOrder := file.ByteOrder
	require.NoError(t, file.Close())

	data, err := os.ReadFile(executable)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(data), 8)
	byteOrder.PutUint32(data[4:8], uint32(macho.CpuPpc64))

	path := filepath.Join(t.TempDir(), "unsupported-worker")
	require.NoError(t, os.WriteFile(path, data, 0o700))
	requireSingleTerminalStartAttempt(t, config{command: path}, nil, syscall.EBADARCH)
}

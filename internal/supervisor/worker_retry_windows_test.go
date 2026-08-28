package supervisor

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows"
)

func TestTerminalWorkerStartErrorRecognizesWindowsLaunchErrors(t *testing.T) {
	tests := map[string]error{
		"bad executable format": windows.ERROR_BAD_EXE_FORMAT,
		"invalid directory":     windows.ERROR_DIRECTORY,
		"invalid parameter":     windows.ERROR_INVALID_PARAMETER,
		"command line too long": windows.ERROR_FILENAME_EXCED_RANGE,
		"machine type mismatch": windows.ERROR_EXE_MACHINE_TYPE_MISMATCH,
		"elevation required":    windows.ERROR_ELEVATION_REQUIRED,
	}

	for name, startErr := range tests {
		t.Run(name, func(t *testing.T) {
			err := &os.PathError{
				Op:   "fork/exec",
				Path: "worker.exe",
				Err:  startErr,
			}

			require.True(t, terminalWorkerStartError(err))
		})
	}
}

func TestRegularFileWorkingDirectoryStopsWorkerStartRetries(t *testing.T) {
	executable, err := os.Executable()
	require.NoError(t, err)

	path := filepath.Join(t.TempDir(), "file")
	require.NoError(t, os.WriteFile(path, nil, 0o600))

	requireSingleTerminalStartAttempt(t, config{
		command: executable,
		dir:     path,
	}, nil, windows.ERROR_DIRECTORY)
}

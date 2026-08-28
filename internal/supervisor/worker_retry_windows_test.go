package supervisor

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows"
)

func TestTerminalWorkerStartErrorRecognizesWindowsLaunchErrors(t *testing.T) {
	tests := map[string]error{
		"bad executable format": windows.ERROR_BAD_EXE_FORMAT,
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

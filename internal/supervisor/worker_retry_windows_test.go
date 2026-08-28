package supervisor

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows"
)

func TestTerminalWorkerStartErrorRecognizesWindowsBadExecutable(t *testing.T) {
	err := &os.PathError{
		Op:   "fork/exec",
		Path: "worker.exe",
		Err:  windows.ERROR_BAD_EXE_FORMAT,
	}

	require.True(t, terminalWorkerStartError(err))
}

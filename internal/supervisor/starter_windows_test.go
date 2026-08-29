package supervisor

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCommandForValidationPreservesRootRelativeWindowsPath(t *testing.T) {
	const command = `\worker.exe`
	require.Equal(t, command, commandForValidation(command, `D:\svc`))
}

func TestCommandForValidationResolvesDriveRelativeWindowsPathOnSameVolume(t *testing.T) {
	require.Equal(t, `C:\svc\bin\worker.exe`, commandForValidation(`c:bin\worker.exe`, `C:\svc`))
}

func TestCommandForValidationPreservesDriveRelativeWindowsPathOnDifferentVolume(t *testing.T) {
	const command = `C:bin\worker.exe`
	require.Equal(t, command, commandForValidation(command, `D:\svc`))
}

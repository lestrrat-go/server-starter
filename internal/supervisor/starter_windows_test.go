package supervisor

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCommandForValidationResolvesRootRelativeWindowsPathFromDirVolume(t *testing.T) {
	const command = `\worker.exe`
	require.Equal(t, `D:\worker.exe`, commandForValidation(command, `D:\svc`))
}

func TestCommandForValidationResolvesDriveRelativeWindowsPathOnSameVolume(t *testing.T) {
	require.Equal(t, `C:\svc\bin\worker.exe`, commandForValidation(`c:bin\worker.exe`, `C:\svc`))
}

func TestCommandForValidationPreservesDriveRelativeWindowsPathOnDifferentVolume(t *testing.T) {
	const command = `C:bin\worker.exe`
	require.Equal(t, command, commandForValidation(command, `D:\svc`))
}

func TestNewStarterCarriesDriveRelativeLookupPathToWorkerCommand(t *testing.T) {
	dir := t.TempDir()
	command := filepath.VolumeName(dir) + `worker`
	executable := filepath.Join(dir, "worker.EXE")
	require.NoError(t, os.WriteFile(executable, nil, 0700))
	t.Setenv("PATHEXT", ".EXE")

	sd, err := NewStarter(&config{
		command: command,
		dir:     dir,
	})
	require.NoError(t, err)

	cmd := sd.workerCommand(context.Background())
	require.Equal(t, command, cmd.Args[0])
	require.Equal(t, executable, cmd.Path)
}

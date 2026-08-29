package supervisor

import (
	"context"
	"os"
	"path/filepath"
	"strings"
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

func TestNeedsValidatedCommandPathPinsRelativeWindowsCommandsUnderUNCDir(t *testing.T) {
	const dir = `\\server\share\service`
	for _, command := range []string{`.\bin\worker.exe`, `..\bin\worker.exe`} {
		t.Run(command, func(t *testing.T) {
			validationCommand := commandForValidation(command, dir)
			require.True(t, needsValidatedCommandPath(command, validationCommand))
		})
	}
}

func TestNewStarterRunsDotRelativeCommandFromDoubleSeparatorDir(t *testing.T) {
	executable, err := os.Executable()
	require.NoError(t, err)

	dir := filepath.Dir(executable)
	if strings.HasPrefix(dir, `\\`) && !strings.HasPrefix(dir, `\\?\`) {
		dir = `\\?\UNC\` + strings.TrimPrefix(dir, `\\`)
	} else if !strings.HasPrefix(dir, `\\?\`) {
		dir = `\\?\` + dir
	}

	command := `.\` + filepath.Base(executable)
	sd, err := NewStarter(&config{
		args:    []string{"-test.run=^$"},
		command: command,
		dir:     dir,
	})
	require.NoError(t, err)

	cmd := sd.workerCommand(t.Context())
	cmd.Dir = sd.dir
	require.Equal(t, command, cmd.Args[0])
	require.Equal(t, filepath.Join(dir, filepath.Base(executable)), cmd.Path)
	// Go rejects a relative application path before CreateProcess when Dir
	// starts with two separators, which is the same path used for a UNC Dir.
	require.NoError(t, cmd.Run())
}

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

func copyCurrentTestExecutable(t *testing.T, target string) {
	t.Helper()

	executable, err := os.Executable()
	require.NoError(t, err)
	data, err := os.ReadFile(executable)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(target, data, 0700))
}

func windowsDoubleSeparatorPath(path string) string {
	if strings.HasPrefix(path, `\\`) && !strings.HasPrefix(path, `\\?\`) {
		return `\\?\UNC\` + strings.TrimPrefix(path, `\\`)
	}
	if !strings.HasPrefix(path, `\\?\`) {
		return `\\?\` + path
	}
	return path
}

func TestNewStarterPinsDriveRelativeWindowsCommandToRelativeDir(t *testing.T) {
	root := t.TempDir()
	binDir := filepath.Join(root, "svc", "bin")
	require.NoError(t, os.MkdirAll(binDir, 0755))
	executable := filepath.Join(binDir, "worker.exe")
	copyCurrentTestExecutable(t, executable)
	t.Chdir(root)

	volume := filepath.VolumeName(root)
	require.NotEmpty(t, volume)
	testCases := []struct {
		name    string
		command string
		pathext string
	}{
		{name: "explicit extension", command: volume + `bin\worker.exe`},
		{name: "PATHEXT lookup", command: volume + `bin\worker`, pathext: ".EXE"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.pathext != "" {
				t.Setenv("PATHEXT", tc.pathext)
			}

			sd, err := NewStarter(&config{
				args:    []string{"-test.run=^$"},
				command: tc.command,
				dir:     "svc",
			})
			require.NoError(t, err)

			cmd := sd.workerCommand(t.Context())
			cmd.Dir = sd.dir
			require.Equal(t, tc.command, cmd.Args[0])
			require.True(t, strings.EqualFold(executable, cmd.Path))
			require.NoError(t, cmd.Run())
		})
	}
}

func TestNewStarterPinsDriveRelativeWindowsCommandUnderDoubleSeparatorDir(t *testing.T) {
	executable, err := os.Executable()
	require.NoError(t, err)

	currentDir := filepath.Dir(filepath.Dir(executable))
	relativeExecutable, err := filepath.Rel(currentDir, executable)
	require.NoError(t, err)
	t.Chdir(currentDir)

	volume := filepath.VolumeName(executable)
	require.NotEmpty(t, volume)
	explicitCommand := volume + relativeExecutable
	testCases := []struct {
		name    string
		command string
		pathext string
	}{
		{name: "explicit extension", command: explicitCommand},
		{
			name:    "PATHEXT lookup",
			command: strings.TrimSuffix(explicitCommand, filepath.Ext(explicitCommand)),
			pathext: ".EXE",
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.pathext != "" {
				t.Setenv("PATHEXT", tc.pathext)
			}

			sd, err := NewStarter(&config{
				args:    []string{"-test.run=^$"},
				command: tc.command,
				dir:     windowsDoubleSeparatorPath(t.TempDir()),
			})
			require.NoError(t, err)

			cmd := sd.workerCommand(t.Context())
			cmd.Dir = sd.dir
			require.Equal(t, tc.command, cmd.Args[0])
			require.True(t, strings.EqualFold(executable, cmd.Path))
			// Go rejects a drive-relative application path before CreateProcess
			// when Dir starts with two separators, including for a UNC Dir.
			require.NoError(t, cmd.Run())
		})
	}
}

func TestNewStarterRunsDotRelativeCommandFromDoubleSeparatorDir(t *testing.T) {
	executable, err := os.Executable()
	require.NoError(t, err)

	dir := windowsDoubleSeparatorPath(filepath.Dir(executable))

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

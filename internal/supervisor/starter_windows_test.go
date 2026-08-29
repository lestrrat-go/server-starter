package supervisor

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewStarterPinsDriveRelativeWindowsCommandToRelativeDir(t *testing.T) {
	root := t.TempDir()
	binDir := filepath.Join(root, "svc", "bin")
	require.NoError(t, os.MkdirAll(binDir, 0755))
	executable := filepath.Join(binDir, "worker.exe")
	testExecutable, err := os.Executable()
	require.NoError(t, err)
	data, err := os.ReadFile(testExecutable)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(executable, data, 0700))
	chdirForTest(t, root)
	t.Setenv("PATHEXT", ".EXE")

	command := filepath.VolumeName(root) + `bin\worker`
	sd, err := NewStarter(&config{
		args:    []string{"-test.run=^$"},
		command: command,
		dir:     "svc",
	})
	require.NoError(t, err)

	cmd := sd.workerCommand(context.Background())
	cmd.Dir = sd.dir
	require.Equal(t, command, cmd.Args[0])
	require.True(t, strings.EqualFold(executable, cmd.Path))
	require.NoError(t, cmd.Run())
}

func TestNewStarterPinsDriveRelativeWindowsCommandForDoubleSeparatorDir(t *testing.T) {
	executable, err := os.Executable()
	require.NoError(t, err)

	currentDir := filepath.Dir(filepath.Dir(executable))
	relativeExecutable, err := filepath.Rel(currentDir, executable)
	require.NoError(t, err)
	chdirForTest(t, currentDir)

	command := filepath.VolumeName(executable) + relativeExecutable
	sd, err := NewStarter(&config{
		args:    []string{"-test.run=^$"},
		command: command,
		dir:     `\\?\` + t.TempDir(),
	})
	require.NoError(t, err)

	cmd := sd.workerCommand(context.Background())
	cmd.Dir = sd.dir
	require.Equal(t, command, cmd.Args[0])
	require.True(t, strings.EqualFold(executable, cmd.Path))
	require.NoError(t, cmd.Run())
}

func chdirForTest(t *testing.T, dir string) {
	t.Helper()

	originalDir, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() {
		require.NoError(t, os.Chdir(originalDir))
	})
}

//go:build !linux && !windows

package statefile_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/lestrrat-go/server-starter/v2/internal/statefile"
	"github.com/stretchr/testify/require"
)

const renamedPIDFileHelperEnv = "SERVER_STARTER_RENAMED_PID_FILE_HELPER"

func TestAcquirePIDFileLockSurvivesParentRename(t *testing.T) {
	root := t.TempDir()
	originalDir := filepath.Join(root, "original")
	require.NoError(t, os.Mkdir(originalDir, 0700))
	originalPath := filepath.Join(originalDir, "server.pid")
	owner, err := statefile.Acquire(originalPath)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, owner.Close()) })

	movedDir := filepath.Join(root, "moved")
	require.NoError(t, os.Rename(originalDir, movedDir))
	movedPath := filepath.Join(movedDir, "server.pid")
	cmd := exec.Command(os.Args[0], "-test.run=^TestAcquirePIDFileAfterParentRenameHelper$")
	cmd.Env = append(os.Environ(), renamedPIDFileHelperEnv+"="+movedPath)
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, string(output))
}

func TestAcquirePIDFileAfterParentRenameHelper(t *testing.T) {
	path := os.Getenv(renamedPIDFileHelperEnv)
	if path == "" {
		return
	}
	contender, err := statefile.Acquire(path)
	if contender != nil {
		_ = contender.Close()
	}
	require.ErrorIs(t, err, statefile.ErrPIDFileLocked)
}

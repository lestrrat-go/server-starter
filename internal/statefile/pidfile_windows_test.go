package statefile

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows"
)

const legacyPIDFileHelper = "SERVER_STARTER_LEGACY_PID_FILE_HELPER"

func TestAcquirePIDFileRejectsLegacyWindowsLock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "server.pid")
	cmd := exec.Command(os.Args[0], "-test.run=^TestLegacyPIDFileLockHelper$")
	cmd.Env = append(os.Environ(), legacyPIDFileHelper+"="+path)
	stdin, err := cmd.StdinPipe()
	require.NoError(t, err)
	stdout, err := cmd.StdoutPipe()
	require.NoError(t, err)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	require.NoError(t, cmd.Start())

	stopped := false
	t.Cleanup(func() {
		if stopped {
			return
		}
		_ = stdin.Close()
		if err := cmd.Wait(); err != nil {
			t.Errorf("legacy PID file helper failed: %v: %s", err, stderr.String())
		}
	})

	ready, err := bufio.NewReader(stdout).ReadString('\n')
	require.NoError(t, err, stderr.String())
	require.Equal(t, "ready\n", ready, stderr.String())

	contender, err := Acquire(path)
	require.Nil(t, contender)
	require.ErrorContains(t, err, path)
	require.ErrorContains(t, err, "already locked")

	require.NoError(t, stdin.Close())
	require.NoError(t, cmd.Wait(), stderr.String())
	stopped = true
}

func TestLegacyPIDFileLockHelper(t *testing.T) {
	path := os.Getenv(legacyPIDFileHelper)
	if path == "" {
		return
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0644)
	require.NoError(t, err)
	defer f.Close()

	var overlapped windows.Overlapped
	require.NoError(t, windows.LockFileEx(
		windows.Handle(f.Fd()),
		windows.LOCKFILE_EXCLUSIVE_LOCK,
		0,
		1,
		0,
		&overlapped,
	))
	require.NoError(t, f.Truncate(0))
	_, err = fmt.Fprintf(f, "%d\n", os.Getpid())
	require.NoError(t, err)
	require.NoError(t, f.Sync())

	_, err = fmt.Fprintln(os.Stdout, "ready")
	require.NoError(t, err)
	_, err = io.Copy(io.Discard, os.Stdin)
	require.NoError(t, err)
}

func TestCurrentWindowsLockKeepsOwnerPIDReadable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "server.pid")
	owner, err := Acquire(path)
	require.NoError(t, err)
	defer owner.Close()

	contender, err := Acquire(path)
	require.Nil(t, contender)
	require.Error(t, err)
	require.Contains(t, err.Error(), "locked by process "+strconv.Itoa(os.Getpid()))
}

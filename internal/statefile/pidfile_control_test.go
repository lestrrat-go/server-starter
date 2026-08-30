//go:build linux

package statefile

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const legacyPIDFileHelperEnv = "SERVER_STARTER_LEGACY_PID_FILE_HELPER"

func TestLegacyPIDFileCanStillBeControlled(t *testing.T) {
	path := filepath.Join(t.TempDir(), "server.pid")
	cmd := exec.CommandContext(context.Background(), os.Args[0], "-test.run=^TestLegacyPIDFileHelper$")
	cmd.Env = append(os.Environ(), legacyPIDFileHelperEnv+"="+path)
	require.NoError(t, cmd.Start())
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})

	requireLegacyPIDFileReady(t, path, cmd.Process.Pid)

	running, err := OpenRunningPID(path)
	require.NoError(t, err)
	require.Equal(t, cmd.Process.Pid, running.PID())
	require.NoError(t, running.Close())
}

func TestReadableLegacyPIDFileCanStillBeControlled(t *testing.T) {
	path := filepath.Join(t.TempDir(), "server.pid")
	cmd := exec.CommandContext(context.Background(), os.Args[0], "-test.run=^TestLegacyPIDFileHelper$")
	cmd.Env = append(os.Environ(), legacyPIDFileHelperEnv+"="+path)
	require.NoError(t, cmd.Start())
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})

	requireLegacyPIDFileReady(t, path, cmd.Process.Pid)
	require.NoError(t, os.Chmod(path, 0400))

	running, err := OpenRunningPID(path)
	require.NoError(t, err)
	require.Equal(t, cmd.Process.Pid, running.PID())
	exited, err := running.Exited()
	require.NoError(t, err)
	require.False(t, exited)
	require.NoError(t, cmd.Process.Kill())
	require.Error(t, cmd.Wait())
	require.Eventually(t, func() bool {
		exited, exitErr := running.Exited()
		return exitErr == nil && exited
	}, time.Second, 10*time.Millisecond)
	require.NoError(t, running.Close())
}

func TestOpenRunningPIDSupportsExecuteOnlyAncestor(t *testing.T) {
	dir := t.TempDir()
	ancestor := filepath.Join(dir, "supervisor")
	require.NoError(t, os.Mkdir(ancestor, 0700))
	path := filepath.Join(ancestor, "server.pid")
	cmd := exec.CommandContext(context.Background(), os.Args[0], "-test.run=^TestLegacyPIDFileHelper$")
	cmd.Env = append(os.Environ(), legacyPIDFileHelperEnv+"="+path)
	require.NoError(t, cmd.Start())
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})

	requireLegacyPIDFileReady(t, path, cmd.Process.Pid)
	require.NoError(t, os.Chmod(ancestor, 0100))
	t.Cleanup(func() { _ = os.Chmod(ancestor, 0700) })

	running, err := OpenRunningPID(path)
	require.NoError(t, err)
	require.Equal(t, cmd.Process.Pid, running.PID())
	require.NoError(t, running.Close())
}

func TestOpenRunningPIDRejectsMismatchedLockOwner(t *testing.T) {
	path := filepath.Join(t.TempDir(), "server.pid")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	require.NoError(t, err)
	t.Cleanup(func() { _ = f.Close() })
	require.NoError(t, syscall.Flock(int(f.Fd()), syscall.LOCK_EX))
	_, err = f.WriteString(strconv.Itoa(os.Getpid() + 1))
	require.NoError(t, err)
	require.NoError(t, f.Sync())

	_, err = OpenRunningPID(path)
	require.Error(t, err)
	require.Contains(t, err.Error(), "does not match lock owner")
}

func requireLegacyPIDFileReady(t *testing.T, path string, pid int) {
	t.Helper()
	require.Eventually(t, func() bool {
		running, err := OpenRunningPID(path)
		if err != nil {
			return false
		}
		matches := running.PID() == pid
		closed := running.Close()
		return matches && closed == nil
	}, time.Second, 10*time.Millisecond)
}

func TestLegacyPIDFileHelper(t *testing.T) {
	path := os.Getenv(legacyPIDFileHelperEnv)
	if path == "" {
		return
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(strconv.Itoa(os.Getpid()) + "\n"); err != nil {
		t.Fatal(err)
	}
	if err := f.Sync(); err != nil {
		t.Fatal(err)
	}
	select {}
}

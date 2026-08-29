//go:build !windows

package cli

import (
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/lestrrat-go/server-starter/v2/internal/statefile"
	"github.com/stretchr/testify/require"
)

func TestDaemonSignalDuringReadinessStopsWorker(t *testing.T) {
	binary := buildStartServer(t)
	dir := t.TempDir()
	supervisorPIDFile := filepath.Join(dir, "supervisor.pid")
	workerPIDFile := filepath.Join(dir, "worker.pid")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary,
		"--daemonize", "--interval", "5", "--pid-file", supervisorPIDFile,
		"--", "/bin/sh", "-c", `printf '%d' $$ > "$1"; trap 'exit 0' TERM; while :; do sleep 1; done`,
		"worker", workerPIDFile,
	)
	require.NoError(t, cmd.Start())

	waitForPIDFile(t, workerPIDFile)
	supervisorPID := readPIDFile(t, supervisorPIDFile)
	require.NoError(t, syscall.Kill(supervisorPID, syscall.SIGTERM))
	require.Error(t, cmd.Wait())
	waitForProcessExit(t, supervisorPID)
	waitForProcessExit(t, readPIDFile(t, workerPIDFile))
}

func TestDaemonReadinessPipeFailureStopsWorker(t *testing.T) {
	binary := buildStartServer(t)
	dir := t.TempDir()
	supervisorPIDFile := filepath.Join(dir, "supervisor.pid")
	workerPIDFile := filepath.Join(dir, "worker.pid")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary,
		"--daemonize", "--interval", "5", "--pid-file", supervisorPIDFile,
		"--", "/bin/sh", "-c", `printf '%d' $$ > "$1"; trap 'exit 0' TERM; while :; do sleep 1; done`,
		"worker", workerPIDFile,
	)
	require.NoError(t, cmd.Start())
	waitForPIDFile(t, workerPIDFile)
	workerPID := readPIDFile(t, workerPIDFile)
	supervisorPID := readPIDFile(t, supervisorPIDFile)

	require.NoError(t, cmd.Process.Kill())
	require.Error(t, cmd.Wait())
	waitForProcessExit(t, supervisorPID)
	waitForProcessExit(t, workerPID)
}

func buildStartServer(t *testing.T) string {
	t.Helper()

	binary := filepath.Join(t.TempDir(), "start_server")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	build := exec.CommandContext(ctx, "go", "build", "-buildvcs=false", "-o", binary, "../../cmd/start_server")
	output, err := build.CombinedOutput()
	require.NoError(t, err, string(output))
	return binary
}

func waitForPIDFile(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if pid, err := statefile.ReadPID(context.Background(), path); err == nil && pid > 0 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", path)
}

func readPIDFile(t *testing.T, path string) int {
	t.Helper()
	pid, err := statefile.ReadPID(context.Background(), path)
	require.NoError(t, err)
	require.Positive(t, pid)
	return pid
}

func waitForProcessExit(t *testing.T, pid int) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		err := syscall.Kill(pid, 0)
		if errors.Is(err, syscall.ESRCH) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("process %d remained alive", pid)
}

//go:build !windows

package cli

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/lestrrat-go/server-starter/v2/internal/control"
	"github.com/lestrrat-go/server-starter/v2/internal/statefile"
	"github.com/stretchr/testify/require"
)

const daemonTestModeEnv = "SERVER_STARTER_DAEMON_TEST_MODE"
const daemonizeFlag = "--daemonize"

func TestDaemonizeWaitsForReadiness(t *testing.T) {
	if os.Getenv(daemonTestModeEnv) == "ready" && os.Getenv(daemonizedEnv) == "1" {
		readiness, err := childDaemonReadiness()
		require.NoError(t, err)
		time.Sleep(150 * time.Millisecond)
		require.NoError(t, readiness.ready())
		return
	}

	t.Setenv(daemonTestModeEnv, "ready")
	t.Setenv(daemonizedEnv, "0")
	t.Setenv(daemonReadinessEnv, "99")
	started := time.Now()
	require.NoError(t, runDaemonizeTest(t, "TestDaemonizeWaitsForReadiness"))
	require.GreaterOrEqual(t, time.Since(started), 100*time.Millisecond)
}

func TestDaemonizeReturnsChildStartupFailure(t *testing.T) {
	if os.Getenv(daemonTestModeEnv) == "failed" && os.Getenv(daemonizedEnv) == "1" {
		readiness, err := childDaemonReadiness()
		require.NoError(t, err)
		readiness.failed(errors.New("invalid listener configuration"))
		return
	}

	t.Setenv(daemonTestModeEnv, "failed")
	require.EqualError(
		t,
		runDaemonizeTest(t, "TestDaemonizeReturnsChildStartupFailure"),
		"daemon startup failed: invalid listener configuration",
	)
}

func TestDaemonizeReturnsCompleteLargeChildStartupFailure(t *testing.T) {
	mode := os.Getenv(daemonTestModeEnv)
	if strings.HasPrefix(mode, "failed-size-") && os.Getenv(daemonizedEnv) == "1" {
		size, err := strconv.Atoi(strings.TrimPrefix(mode, "failed-size-"))
		require.NoError(t, err)
		readiness, err := childDaemonReadiness()
		require.NoError(t, err)
		readiness.failed(errors.New(strings.Repeat("x", size)))
		return
	}

	for _, size := range []int{64 * 1024, 70_000} {
		t.Run(strconv.Itoa(size), func(t *testing.T) {
			t.Setenv(daemonTestModeEnv, "failed-size-"+strconv.Itoa(size))
			message := strings.Repeat("x", size)
			statusErr := runDaemonizeTest(t, "TestDaemonizeReturnsCompleteLargeChildStartupFailure")
			require.Error(t, statusErr)
			want := "daemon startup failed: " + message
			require.Len(t, statusErr.Error(), len(want))
			require.Equal(t, sha256.Sum256([]byte(want)), sha256.Sum256([]byte(statusErr.Error())))
		})
	}
}

func runDaemonizeTest(t *testing.T, name string) error {
	t.Helper()

	// Restrict the daemon child to its helper test so unrelated tests cannot
	// consume the readiness descriptor before the intended child branch.
	originalArgs := os.Args
	os.Args = []string{os.Args[0], "-test.run=^" + name + "$"}
	defer func() {
		os.Args = originalArgs
	}()

	return daemonize()
}

func TestRunDaemonizeReportsStartupFailures(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	binary := filepath.Join(t.TempDir(), "start_server")
	build := exec.CommandContext(ctx, "go", "build", "-o", binary, "../../cmd/start_server")
	buildOutput, err := build.CombinedOutput()
	require.NoError(t, err, string(buildOutput))

	missingWorkerDir := filepath.Join(t.TempDir(), "missing")
	testCases := []struct {
		name      string
		args      []string
		want      string
		fileLimit bool
	}{
		{
			name: "missing command",
			args: []string{daemonizeFlag},
			want: "daemon startup failed: server program not specified",
		},
		{
			name: "log file cannot be opened",
			args: []string{daemonizeFlag, "--log-file", t.TempDir(), "--", os.Args[0]},
			want: "daemon startup failed: open ",
		},
		{
			name: "listener configuration is invalid",
			args: []string{daemonizeFlag, "--port", "not-a-port", "--", os.Args[0]},
			want: "daemon startup failed: invalid port",
		},
		{
			name: "worker directory does not exist",
			args: []string{daemonizeFlag, "--dir", missingWorkerDir, "--", "/bin/true"},
			want: fmt.Sprintf("daemon startup failed: chdir %s: no such file or directory", missingWorkerDir),
		},
		{
			name:      "worker descriptor setup fails",
			args:      []string{daemonizeFlag, "--port", "0=63", "--dir", missingWorkerDir, "--", "/bin/true"},
			want:      "daemon startup failed: open worker descriptor padding: open /dev/null: too many open files",
			fileLimit: true,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cmd := exec.CommandContext(ctx, binary, tc.args...)
			if tc.fileLimit {
				args := append([]string{"-c", `ulimit -n 64; exec "$@"`, "sh", binary}, tc.args...)
				cmd = exec.CommandContext(ctx, "/bin/sh", args...)
			}
			output, err := cmd.CombinedOutput()
			require.Error(t, err)
			require.Contains(t, strings.TrimSpace(string(output)), tc.want)
		})
	}
}

func TestDaemonizeDoesNotLeakReadinessToNestedStartServer(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	testDir := t.TempDir()
	binary := filepath.Join(testDir, "start_server")
	build := exec.CommandContext(ctx, "go", "build", "-o", binary, "../../cmd/start_server")
	buildOutput, err := build.CombinedOutput()
	require.NoError(t, err, string(buildOutput))

	outerPIDFile := filepath.Join(testDir, "outer.pid")
	innerPIDFile := filepath.Join(testDir, "inner.pid")
	t.Cleanup(func() {
		if _, err := os.Stat(outerPIDFile); errors.Is(err, os.ErrNotExist) {
			return
		}
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer stopCancel()
		require.NoError(t, control.Stop(stopCtx, outerPIDFile))
	})

	cmd := exec.CommandContext(
		ctx,
		binary,
		"--daemonize",
		"--interval", "1",
		"--pid-file", outerPIDFile,
		"--",
		binary,
		"--interval", "0",
		"--pid-file", innerPIDFile,
		"--",
		"/bin/sleep", "30",
	)
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, string(output))

	innerPID, err := statefile.ReadPID(ctx, innerPIDFile)
	require.NoError(t, err)
	require.Positive(t, innerPID)
}

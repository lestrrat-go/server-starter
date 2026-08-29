//go:build !windows

package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const daemonTestModeEnv = "SERVER_STARTER_DAEMON_TEST_MODE"

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
	require.NoError(t, daemonize())
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
	require.EqualError(t, daemonize(), "daemon startup failed: invalid listener configuration")
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
		name string
		args []string
		want string
	}{
		{
			name: "missing command",
			args: []string{"--daemonize"},
			want: "daemon startup failed: server program not specified",
		},
		{
			name: "log file cannot be opened",
			args: []string{"--daemonize", "--log-file", t.TempDir(), "--", os.Args[0]},
			want: "daemon startup failed: open ",
		},
		{
			name: "listener configuration is invalid",
			args: []string{"--daemonize", "--port", "not-a-port", "--", os.Args[0]},
			want: "daemon startup failed: invalid port",
		},
		{
			name: "worker directory does not exist",
			args: []string{"--daemonize", "--dir", missingWorkerDir, "--", "/bin/true"},
			want: fmt.Sprintf("daemon startup failed: chdir %s: no such file or directory", missingWorkerDir),
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cmd := exec.CommandContext(ctx, binary, tc.args...)
			output, err := cmd.CombinedOutput()
			require.Error(t, err)
			require.Contains(t, strings.TrimSpace(string(output)), tc.want)
		})
	}
}

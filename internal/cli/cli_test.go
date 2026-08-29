package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const cliTestHelperEnv = "SERVER_STARTER_CLI_TEST_HELPER"
const invalidDaemonSignalEnv = "SERVER_STARTER_TEST_INVALID_DAEMON_SIGNAL"

func TestRunRejectsInvalidSignalOptions(t *testing.T) {
	if os.Getenv(cliTestHelperEnv) == "1" {
		separator := slices.Index(os.Args, "--")
		if separator < 0 {
			os.Exit(2)
		}
		os.Args = append([]string{"start_server"}, os.Args[separator+1:]...)
		os.Exit(Run())
	}

	for _, option := range []string{"--signal-on-hup", "--signal-on-term"} {
		t.Run(option, func(t *testing.T) {
			value := "NOTASIGNAL"
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestRunRejectsInvalidSignalOptions$", "--",
				fmt.Sprintf("%s=%s", option, value), "--", "unused-command")
			cmd.Env = append(os.Environ(), cliTestHelperEnv+"=1")
			output, err := cmd.CombinedOutput()

			var exitErr *exec.ExitError
			require.ErrorAs(t, err, &exitErr)
			require.Equal(t, 1, exitErr.ExitCode())
			require.Equal(t,
				fmt.Sprintf("error: invalid %s value %q: unknown signal\n", option, value),
				string(output),
			)
		})
	}
}

func TestDaemonizeRejectsInvalidSignalNames(t *testing.T) {
	if flag := os.Getenv(invalidDaemonSignalEnv); flag != "" {
		os.Args = []string{os.Args[0], "--daemonize", flag, "TERMM", "--", os.Args[0]}
		os.Exit(Run())
	}

	testCases := []struct {
		name     string
		flag     string
		expected string
	}{
		{
			name:     "signal on HUP",
			flag:     "--signal-on-hup",
			expected: "error: invalid --signal-on-hup value \"TERMM\": unknown signal\n",
		},
		{
			name:     "signal on TERM",
			flag:     "--signal-on-term",
			expected: "error: invalid --signal-on-term value \"TERMM\": unknown signal\n",
		},
	}

	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestDaemonizeRejectsInvalidSignalNames$")
			cmd.Env = append(os.Environ(), invalidDaemonSignalEnv+"="+test.flag)
			output, err := cmd.CombinedOutput()

			var exitErr *exec.ExitError
			require.ErrorAs(t, err, &exitErr)
			require.Equal(t, 1, exitErr.ExitCode())
			require.Equal(t, test.expected, string(output))
		})
	}
}

func TestValidateControlActions(t *testing.T) {
	testCases := []struct {
		name    string
		opts    options
		wantErr string
	}{
		{name: "no control action"},
		{name: "stop", opts: options{OptStop: true}},
		{name: "restart", opts: options{OptRestart: true}},
		{
			name:    "stop and restart",
			opts:    options{OptStop: true, OptRestart: true},
			wantErr: "--stop and --restart cannot be used together; choose one action",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateControlActions(&tc.opts)
			if tc.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.EqualError(t, err, tc.wantErr)
		})
	}
}

func TestRunRejectsConflictingControlActions(t *testing.T) {
	readStderr, writeStderr, err := os.Pipe()
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, readStderr.Close())
	})

	originalArgs := os.Args
	originalStderr := os.Stderr
	os.Args = []string{"start_server", "--stop", "--restart"}
	os.Stderr = writeStderr
	t.Cleanup(func() {
		os.Args = originalArgs
		os.Stderr = originalStderr
	})

	exitCode := Run()
	require.NoError(t, writeStderr.Close())
	os.Stderr = originalStderr

	stderr, err := io.ReadAll(readStderr)
	require.NoError(t, err)
	require.Equal(t, 1, exitCode)
	require.Equal(t, "--stop and --restart cannot be used together; choose one action\n", string(stderr))
}

func TestRunRejectsInvalidSettingsBeforeDaemonize(t *testing.T) {
	t.Setenv("ENABLE_AUTO_RESTART", "banana")

	daemonizeCalled := false
	exitCode, stdout, stderr := runCLIWithDaemonize(t, func() error {
		daemonizeCalled = true
		return nil
	}, "--daemonize", "/bin/true")

	require.Equal(t, 1, exitCode)
	require.Empty(t, stdout)
	require.Contains(t, stderr, `error: invalid ENABLE_AUTO_RESTART value "banana"`)
	require.False(t, daemonizeCalled)
}

func TestRunInformationalAndParseExitCodes(t *testing.T) {
	testCases := []struct {
		name       string
		args       []string
		exitCode   int
		stdoutText string
		stderrText string
	}{
		{
			name:       "help succeeds",
			args:       []string{"--help"},
			exitCode:   0,
			stderrText: "Usage:",
		},
		{
			name:       "version succeeds",
			args:       []string{"--version"},
			exitCode:   0,
			stdoutText: "0.0.2\n",
		},
		{
			name:       "parse error fails",
			args:       []string{"--unknown-option"},
			exitCode:   1,
			stderrText: "Usage:",
		},
		{
			name:       "help does not hide a parse error",
			args:       []string{"--help", "--unknown-option"},
			exitCode:   1,
			stderrText: "Usage:",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			exitCode, stdout, stderr := runCLI(t, tc.args...)
			require.Equal(t, tc.exitCode, exitCode)
			if tc.stdoutText == "" {
				require.Empty(t, stdout)
			} else {
				require.Contains(t, stdout, tc.stdoutText)
			}
			if tc.stderrText == "" {
				require.Empty(t, stderr)
			} else {
				require.Contains(t, stderr, tc.stderrText)
			}
		})
	}
}

func runCLI(t *testing.T, args ...string) (int, string, string) {
	return runCLIWithDaemonize(t, daemonize, args...)
}

func runCLIWithDaemonize(
	t *testing.T,
	daemonizeFn func() error,
	args ...string,
) (int, string, string) {
	t.Helper()

	stdoutReader, stdoutWriter, err := os.Pipe()
	require.NoError(t, err)
	stderrReader, stderrWriter, err := os.Pipe()
	require.NoError(t, err)

	originalArgs := os.Args
	originalStdout := os.Stdout
	originalStderr := os.Stderr
	os.Args = append([]string{"start_server"}, args...)
	os.Stdout = stdoutWriter
	os.Stderr = stderrWriter
	t.Cleanup(func() {
		os.Args = originalArgs
		os.Stdout = originalStdout
		os.Stderr = originalStderr
	})

	exitCode := run(daemonizeFn)
	require.NoError(t, stdoutWriter.Close())
	require.NoError(t, stderrWriter.Close())

	stdout, err := io.ReadAll(stdoutReader)
	require.NoError(t, err)
	stderr, err := io.ReadAll(stderrReader)
	require.NoError(t, err)
	require.NoError(t, stdoutReader.Close())
	require.NoError(t, stderrReader.Close())

	return exitCode, string(stdout), string(stderr)
}

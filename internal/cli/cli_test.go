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

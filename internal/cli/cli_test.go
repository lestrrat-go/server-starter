package cli

import (
	"context"
	"fmt"
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

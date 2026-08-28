package cli_test

import (
	"io"
	"os"
	"testing"

	"github.com/lestrrat-go/server-starter/v2/internal/cli"
	"github.com/stretchr/testify/require"
)

func TestRunReportsPrereleaseVersion(t *testing.T) {
	originalArgs := os.Args
	os.Args = []string{"start_server", "--version"}
	t.Cleanup(func() { os.Args = originalArgs })

	stdout, stdoutWriter, err := os.Pipe()
	require.NoError(t, err)

	originalStdout := os.Stdout
	os.Stdout = stdoutWriter
	t.Cleanup(func() { os.Stdout = originalStdout })

	exitCode := cli.Run()
	os.Stdout = originalStdout
	require.NoError(t, stdoutWriter.Close())

	output, err := io.ReadAll(stdout)
	require.NoError(t, err)
	require.NoError(t, stdout.Close())
	require.Equal(t, 0, exitCode)
	require.Equal(t, "2.0.0-dev\n", string(output))
}

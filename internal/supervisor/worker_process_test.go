package supervisor

import (
	"fmt"
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const testWorkerArg = "server-starter-test-worker"

func testWorkerPorts() []string {
	if runtime.GOOS == testGOOSWindows {
		return nil
	}
	return []string{"0"}
}

func TestWorkerPortsForPlatform(t *testing.T) {
	if runtime.GOOS == testGOOSWindows {
		require.Empty(t, testWorkerPorts())
		return
	}
	require.Equal(t, []string{"0"}, testWorkerPorts())
}

func testWorkerCommand(t *testing.T, args ...string) (string, []string) {
	t.Helper()

	executable, err := os.Executable()
	require.NoError(t, err)

	workerArgs := []string{"-test.run=^TestSupervisorWorkerProcess$", "--", testWorkerArg}
	workerArgs = append(workerArgs, args...)
	return executable, workerArgs
}

func TestSupervisorWorkerProcess(t *testing.T) {
	argIndex := -1
	for i, arg := range os.Args {
		if arg == testWorkerArg {
			argIndex = i
			break
		}
	}
	if argIndex < 0 {
		return
	}

	args := os.Args[argIndex+1:]
	if len(args) > 0 {
		marker, err := os.OpenFile(args[0], os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
		require.NoError(t, err)
		_, err = fmt.Fprintln(marker, os.Getpid())
		require.NoError(t, err)
		require.NoError(t, marker.Close())
	}

	for {
		time.Sleep(time.Hour)
	}
}

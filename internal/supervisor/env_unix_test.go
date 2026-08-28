//go:build !windows

package supervisor

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

func TestReloadEnvdirSkipsNamedPipeWithoutBlocking(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, unix.Mkfifo(filepath.Join(dir, "PIPE"), 0600))

	done := make(chan error, 1)
	go func() {
		_, err := reloadEnv(dir)
		done <- err
	}()

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("reloadEnv blocked on a named pipe")
	}
}

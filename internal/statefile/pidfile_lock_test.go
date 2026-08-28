package statefile_test

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/lestrrat-go/server-starter/v2/internal/statefile"
	"github.com/stretchr/testify/require"
)

func TestAcquirePIDFileRejectsHeldLockWithoutWaiting(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "windows" {
		t.Skip("POSIX record locks are shared by file descriptors in one process")
	}

	path := filepath.Join(t.TempDir(), "server.pid")
	owner, err := statefile.Acquire(path)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, owner.Close())
	})

	errCh := make(chan error, 1)
	go func() {
		contender, err := statefile.Acquire(path)
		if contender != nil {
			_ = contender.Close()
		}
		errCh <- err
	}()

	select {
	case err := <-errCh:
		require.ErrorIs(t, err, statefile.ErrPIDFileLocked)
		require.True(t, strings.Contains(err.Error(), path), "error %q does not name pid file", err)
	case <-time.After(time.Second):
		t.Fatal("Acquire blocked while another supervisor held the pid-file lock")
	}

	require.NoError(t, owner.Close())
	replacement, err := statefile.Acquire(path)
	require.NoError(t, err)
	require.NoError(t, replacement.Close())
}

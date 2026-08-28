package statefile_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lestrrat-go/server-starter/v2/internal/statefile"
	"github.com/stretchr/testify/require"
)

func TestOpenRunningPIDEnforcesSigned32BitRange(t *testing.T) {
	for _, value := range []string{"2147483648", "4294967295"} {
		t.Run(value, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "pid")
			require.NoError(t, os.WriteFile(path, []byte(value+"\n"), 0600))

			_, err := statefile.OpenRunningPID(path)
			require.ErrorContains(t, err, "invalid pid file")
		})
	}
}

func TestReadStatusRejectsOversizedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "status")
	require.NoError(t, os.WriteFile(path, []byte(strings.Repeat("1:2\n", 32*1024)), 0600))

	_, err := statefile.ReadStatus(context.Background(), path)
	require.ErrorContains(t, err, "too large")
}

func TestReadStatusRejectsDirectory(t *testing.T) {
	_, err := statefile.ReadStatus(context.Background(), t.TempDir())
	require.ErrorContains(t, err, "not a regular file")
}

func TestReadStatusHonorsCancelledContext(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state")
	require.NoError(t, os.WriteFile(path, []byte("1\n"), 0600))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := statefile.ReadStatus(ctx, path)
	require.ErrorIs(t, err, context.Canceled)
}

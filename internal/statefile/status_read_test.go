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

func TestReadPIDRejectsOversizedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pid")
	require.NoError(t, os.WriteFile(path, []byte(strings.Repeat("1", 1024)), 0600))

	_, err := statefile.ReadPID(context.Background(), path)
	require.ErrorContains(t, err, "too large")
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

func TestReadStateFilesHonorCancelledContext(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state")
	require.NoError(t, os.WriteFile(path, []byte("1\n"), 0600))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := statefile.ReadPID(ctx, path)
	require.ErrorIs(t, err, context.Canceled)

	_, err = statefile.ReadStatus(ctx, path)
	require.ErrorIs(t, err, context.Canceled)
}

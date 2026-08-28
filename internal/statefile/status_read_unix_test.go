//go:build !windows

package statefile_test

import (
	"context"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/lestrrat-go/server-starter/v2/internal/statefile"
	"github.com/stretchr/testify/require"
)

func TestReadStatusRejectsFIFOWithoutBlocking(t *testing.T) {
	path := filepath.Join(t.TempDir(), "status")
	require.NoError(t, syscall.Mkfifo(path, 0600))

	_, err := statefile.ReadStatus(context.Background(), path)
	require.ErrorContains(t, err, "not a regular file")
}

//go:build !windows

package supervisor

import (
	"os"
	"syscall"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSIGXFSZCanonicalName(t *testing.T) {
	require.Equal(t, "XFSZ", signame(syscall.SIGXFSZ))
}

func TestSigFromNameRecognizesXFSZ(t *testing.T) {
	for _, name := range []string{"XFSZ", "SIGXFSZ", "GXFSZ"} {
		got, err := SigFromName(name)
		require.NoError(t, err)
		require.Equal(t, os.Signal(syscall.SIGXFSZ), got)
	}
}

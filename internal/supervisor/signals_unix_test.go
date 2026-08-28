//go:build !windows

package supervisor

import (
	"syscall"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSIGXFSZCanonicalName(t *testing.T) {
	require.Equal(t, "XFSZ", signame(syscall.SIGXFSZ))
}

func TestSigFromNameRecognizesXFSZ(t *testing.T) {
	require.Equal(t, syscall.SIGXFSZ, SigFromName("XFSZ"))
	require.Equal(t, syscall.SIGXFSZ, SigFromName("SIGXFSZ"))
}

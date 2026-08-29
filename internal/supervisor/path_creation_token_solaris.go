//go:build solaris

package supervisor

import (
	"os"

	"golang.org/x/sys/unix"
)

func creationTokenAt(_ *os.File, _ string, _ *unix.Stat_t) string {
	return ""
}

func creationTokenForFile(_ *os.File, _ *unix.Stat_t) string {
	return ""
}

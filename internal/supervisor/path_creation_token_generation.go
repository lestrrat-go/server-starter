//go:build aix || darwin || dragonfly || freebsd || netbsd || openbsd

package supervisor

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

func creationTokenAt(_ *os.File, _ string, stat *unix.Stat_t) string {
	return fmt.Sprintf("generation:%d", stat.Gen)
}

func creationTokenForFile(_ *os.File, stat *unix.Stat_t) string {
	return fmt.Sprintf("generation:%d", stat.Gen)
}

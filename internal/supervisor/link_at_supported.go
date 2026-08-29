//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd

package supervisor

import (
	"os"

	"golang.org/x/sys/unix"
)

func linkAt(oldDir *os.File, oldName string, newDir *os.File, newName string) error {
	return unix.Linkat(int(oldDir.Fd()), oldName, int(newDir.Fd()), newName, 0)
}

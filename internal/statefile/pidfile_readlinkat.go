//go:build linux || darwin || freebsd || netbsd || openbsd

package statefile

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

func readDirectoryLink(parent *os.File, name string) (string, error) {
	buffer := make([]byte, 4096)
	n, err := unix.Readlinkat(int(parent.Fd()), name, buffer)
	if err != nil {
		return "", err
	}
	if n == len(buffer) {
		return "", fmt.Errorf("symbolic link %q is too long", name)
	}
	return string(buffer[:n]), nil
}

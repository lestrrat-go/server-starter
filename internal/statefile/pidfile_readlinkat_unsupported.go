//go:build !windows && !linux && !darwin && !freebsd && !netbsd && !openbsd

package statefile

import (
	"fmt"
	"os"
)

// Some Unix targets do not expose readlinkat through x/sys/unix. Rejecting a
// symlinked ancestor keeps control fail-closed on those targets while leaving
// ordinary, non-symlinked PID paths usable.
func readDirectoryLink(_ *os.File, name string) (string, error) {
	return "", fmt.Errorf("descriptor-relative symbolic link %q is unsupported on this platform", name)
}

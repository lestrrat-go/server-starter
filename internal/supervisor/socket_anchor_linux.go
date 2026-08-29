//go:build linux

package supervisor

import "golang.org/x/sys/unix"

func anchorSocketEntry(path string) (func(), error) {
	fd, err := unix.Open(path, unix.O_PATH|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	return func() { _ = unix.Close(fd) }, nil
}

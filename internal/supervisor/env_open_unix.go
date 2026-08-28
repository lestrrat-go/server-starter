//go:build !windows

package supervisor

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

func openEnvFile(name string) (*os.File, error) {
	fd, err := unix.Open(name, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}

	file := os.NewFile(uintptr(fd), name)
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		_ = file.Close()
		if err != nil {
			return nil, err
		}
		return nil, errors.New("envdir entry is not a regular file")
	}
	return file, nil
}

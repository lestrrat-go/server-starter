//go:build !windows

package statefile

import (
	"fmt"
	"os"
	"syscall"
)

func openPIDFile(path string) (*os.File, error) {
	flags := os.O_RDWR | syscall.O_NOFOLLOW
	f, err := os.OpenFile(path, flags|os.O_CREATE|os.O_EXCL, 0644)
	if os.IsExist(err) {
		f, err = os.OpenFile(path, flags, 0)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to open pid file %q: %w", path, err)
	}

	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("failed to inspect pid file %q: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		f.Close()
		return nil, fmt.Errorf("pid file %q is not a regular file", path)
	}

	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		f.Close()
		return nil, fmt.Errorf("failed to verify ownership of pid file %q", path)
	}
	expectedUID := uint32(os.Geteuid())
	if stat.Uid != expectedUID {
		f.Close()
		return nil, fmt.Errorf("pid file %q is owned by uid %d, expected uid %d", path, stat.Uid, expectedUID)
	}
	if stat.Nlink != 1 {
		f.Close()
		return nil, fmt.Errorf("pid file %q has %d hard links, expected one", path, stat.Nlink)
	}

	return f, nil
}

func lockFile(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_EX)
}

// TryLock attempts to take an exclusive, non-blocking lock on f. It is used
// by control.Stop to poll for the supervisor having exited: once the
// supervisor process dies, its blocking lock on the pid file (see Acquire)
// is released and this call starts succeeding. This is deliberately
// non-blocking, unlike the blocking lock the supervisor itself takes at
// startup.
func TryLock(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
}

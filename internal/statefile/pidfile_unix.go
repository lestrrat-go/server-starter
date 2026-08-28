//go:build !windows

package statefile

import (
	"os"
	"syscall"
)

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

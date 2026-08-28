//go:build !windows

package control

import (
	"os"
	"syscall"
)

// tryLockPIDFile attempts to take an exclusive, non-blocking lock on f. It
// is used by Stop to poll for the supervisor having exited: once the
// supervisor process dies, its blocking lock on the pid file (see
// pidfile_unix.go in the root package) is released and this call starts
// succeeding. This is deliberately non-blocking, unlike the blocking lock
// the supervisor itself takes at startup.
func tryLockPIDFile(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
}

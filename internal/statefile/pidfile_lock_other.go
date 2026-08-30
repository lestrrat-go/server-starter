//go:build !linux && !windows

package statefile

import (
	"fmt"
	"os"
	"syscall"
)

// BSD flock and fcntl locks conflict on supported non-Linux Unix systems.
// Keep the flock lifetime lock so separate opens in one process still contend;
// lockOwnerPID uses the conflicting record-lock query to identify its owner.
func lockFile(f *os.File, _ string) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
}

func lockOwnerPID(f *os.File, _ string) (int, error) {
	lock := pidFileRecordLock()
	if err := syscall.FcntlFlock(f.Fd(), syscall.F_GETLK, &lock); err != nil {
		return 0, err
	}
	if lock.Type == syscall.F_UNLCK {
		return 0, nil
	}
	if lock.Pid <= 0 {
		return 0, fmt.Errorf("record lock has no process owner")
	}
	return int(lock.Pid), nil
}

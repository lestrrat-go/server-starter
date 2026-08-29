//go:build !linux && !windows

package statefile

import (
	"fmt"
	"os"
	"syscall"
)

// BSD flock and fcntl locks conflict on supported non-Linux Unix systems.
// New supervisors therefore use only the inspectable record lock, while
// OpenRunningPID keeps a compatibility path for old flock-only supervisors.
func lockFile(f *os.File, path string) error {
	lock, err := pathRecordLock(path)
	if err != nil {
		return err
	}
	return syscall.FcntlFlock(f.Fd(), syscall.F_SETLK, &lock)
}

func lockOwnerPID(f *os.File, path string) (int, error) {
	lock, err := pathRecordLock(path)
	if err != nil {
		return 0, err
	}
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

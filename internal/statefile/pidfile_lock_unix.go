//go:build !windows

package statefile

import (
	"errors"
	"os"
	"syscall"
)

func lockNoLongerOwnedByPID(f *os.File, pid int) (bool, error) {
	ownerPID, err := lockOwnerPID(f, "")
	if err != nil {
		return false, err
	}
	if ownerPID > 0 {
		return ownerPID != pid, nil
	}

	err = TryLock(f)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, syscall.EACCES) || errors.Is(err, syscall.EAGAIN) {
		return false, nil
	}
	return false, err
}

// pidFileRecordLock covers the inode from byte zero through the end of the
// file, so the lock range remains stable when the containing directory moves.
func pidFileRecordLock() syscall.Flock_t {
	return syscall.Flock_t{Type: syscall.F_WRLCK, Whence: 0, Start: 0, Len: 0}
}

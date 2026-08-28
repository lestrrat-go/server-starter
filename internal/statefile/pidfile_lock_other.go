//go:build !linux && !windows

package statefile

import (
	"errors"
	"os"
	"syscall"
)

func lockFile(f *os.File, path string) error {
	lock, err := pathRecordLock(path)
	if err != nil {
		return err
	}
	return syscall.FcntlFlock(f.Fd(), syscall.F_SETLK, &lock)
}

func inspectInodeLocks(f *os.File) (int, bool, bool, error) {
	recordLock := syscall.Flock_t{Type: syscall.F_WRLCK, Whence: 0, Start: 0, Len: 0}
	if err := syscall.FcntlFlock(f.Fd(), syscall.F_GETLK, &recordLock); err != nil {
		return 0, false, false, err
	}
	hasRecordLock := recordLock.Type != syscall.F_UNLCK

	err := TryLock(f)
	if err == nil {
		if unlockErr := syscall.Flock(int(f.Fd()), syscall.LOCK_UN); unlockErr != nil {
			return 0, false, false, unlockErr
		}
		return 0, hasRecordLock, false, nil
	}
	if !errors.Is(err, syscall.EACCES) && !errors.Is(err, syscall.EAGAIN) {
		return 0, false, false, err
	}
	return 0, hasRecordLock, true, nil
}

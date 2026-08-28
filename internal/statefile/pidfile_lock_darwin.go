package statefile

import (
	"errors"
	"fmt"
	"os"
	"syscall"
)

func inspectInodeLocks(f *os.File, _ int) (int, bool, error) {
	recordLock := syscall.Flock_t{Type: syscall.F_WRLCK, Whence: 0, Start: 0, Len: 0}
	if err := syscall.FcntlFlock(f.Fd(), syscall.F_GETLK, &recordLock); err != nil {
		return 0, false, err
	}
	if recordLock.Type != syscall.F_UNLCK {
		return 0, true, nil
	}

	err := TryLock(f)
	if err == nil {
		if unlockErr := syscall.Flock(int(f.Fd()), syscall.LOCK_UN); unlockErr != nil {
			return 0, false, unlockErr
		}
		return 0, false, nil
	}
	if !errors.Is(err, syscall.EACCES) && !errors.Is(err, syscall.EAGAIN) {
		return 0, false, err
	}
	return 0, false, fmt.Errorf("legacy BSD flock ownership cannot be attributed to a process on darwin")
}

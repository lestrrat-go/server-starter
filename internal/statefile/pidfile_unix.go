//go:build !windows

package statefile

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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

func readPIDText(f *os.File, data []byte) (int, error) {
	return f.ReadAt(data, 0)
}

func lockFile(f *os.File, path string) error {
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return err
	}
	lock, err := pathRecordLock(path)
	if err != nil {
		return err
	}
	return syscall.FcntlFlock(f.Fd(), syscall.F_SETLK, &lock)
}

func finishPIDFileLock(*os.File) error {
	return nil
}

func lockUnavailable(err error) bool {
	return errors.Is(err, syscall.EWOULDBLOCK)
}

func validatePIDFileLinkCount(f *os.File, path string) error {
	info, err := f.Stat()
	if err != nil {
		return fmt.Errorf("failed to inspect pid file %q: %w", path, err)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("failed to verify link count of pid file %q", path)
	}
	if stat.Nlink != 1 {
		return fmt.Errorf("pid file %q has %d hard links, expected one", path, stat.Nlink)
	}
	return nil
}

func closePIDFile(f *os.File, path string) error {
	var removeErr error
	if pathInfo, err := os.Stat(path); err == nil {
		if fileInfo, statErr := f.Stat(); statErr == nil && os.SameFile(pathInfo, fileInfo) {
			if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				removeErr = err
			}
		}
	}
	return errors.Join(removeErr, f.Close())
}

// TryLock attempts to take an exclusive, non-blocking BSD lock on f. It is
// used by control.Stop to poll for a legacy supervisor having exited: once
// the supervisor process dies, its lock on the pid file is released
// and this call starts succeeding. It returns syscall.EWOULDBLOCK while
// another process holds the lock; any other error means the check failed.
func TryLock(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
}

func lockOwnerPID(f *os.File, path string) (int, pidLockKind, error) {
	lock, err := pathRecordLock(path)
	if err != nil {
		return 0, pidLockUnknown, err
	}
	if err := syscall.FcntlFlock(f.Fd(), syscall.F_GETLK, &lock); err != nil {
		return 0, pidLockUnknown, err
	}
	if lock.Type != syscall.F_UNLCK {
		if lock.Pid <= 0 {
			return 0, pidLockUnknown, fmt.Errorf("record lock has no process owner")
		}
		return int(lock.Pid), pidLockRecord, nil
	}

	flockPID, hasRecordLock, err := inspectInodeLocks(f)
	if err != nil {
		return 0, pidLockUnknown, err
	}
	if hasRecordLock {
		return 0, pidLockUnknown, fmt.Errorf("pid file lock was acquired for a different path")
	}
	if flockPID > 0 {
		return flockPID, pidLockFlock, nil
	}
	return 0, pidLockUnknown, nil
}

func lockReleased(f *os.File, path string, kind pidLockKind) (bool, error) {
	var err error
	switch kind {
	case pidLockRecord:
		lock, lockErr := pathRecordLock(path)
		if lockErr != nil {
			return false, lockErr
		}
		err = syscall.FcntlFlock(f.Fd(), syscall.F_SETLK, &lock)
	case pidLockFlock:
		err = TryLock(f)
	default:
		return false, fmt.Errorf("unknown pid-file lock kind")
	}
	if err == nil {
		return true, nil
	}
	if errors.Is(err, syscall.EACCES) || errors.Is(err, syscall.EAGAIN) {
		return false, nil
	}
	return false, err
}

// pathRecordLock binds a supervisor lock to the absolute pid-file path
// without changing the traditional one-line pid-file format. The BSD flock
// remains the lifetime lock; this record lock supplies an inspectable owner
// pid and makes a locked file moved from another path fail validation.
func pathRecordLock(path string) (syscall.Flock_t, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return syscall.Flock_t{}, err
	}
	digest := sha256.Sum256([]byte(filepath.Clean(absPath)))
	start := int64(binary.BigEndian.Uint64(digest[:8]) & uint64(^uint64(0)>>1))
	if start == 0 {
		start = 1
	}
	return syscall.Flock_t{Type: syscall.F_WRLCK, Whence: 0, Start: start, Len: 1}, nil
}

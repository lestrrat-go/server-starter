//go:build !windows

package statefile

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"syscall"
)

func lockReleased(f *os.File) (bool, error) {
	err := TryLock(f)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, syscall.EACCES) || errors.Is(err, syscall.EAGAIN) {
		return false, nil
	}
	return false, err
}

// pathRecordLock adds a pathname-specific byte lock to platforms that expose
// process ownership through fcntl. The digest avoids changing the pid format.
func pathRecordLock(path string) (syscall.Flock_t, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return syscall.Flock_t{}, err
	}
	digest := sha256.Sum256([]byte(filepath.Clean(absPath)))
	start := int64(binary.BigEndian.Uint64(digest[:8]) & (^uint64(0) >> 1))
	if start == 0 {
		start = 1
	}
	return syscall.Flock_t{Type: syscall.F_WRLCK, Whence: 0, Start: start, Len: 1}, nil
}

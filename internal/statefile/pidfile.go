package statefile

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

const pidTextSize = 64

// ErrPIDFileLocked means a live supervisor already holds the pid-file lock.
var ErrPIDFileLocked = errors.New("pid file is already locked")

// PIDFile is a pid file that has been acquired via Acquire. Closing it
// releases the lock and, if this process still owns the file on disk,
// removes it.
type PIDFile struct {
	file *os.File
	path string
}

// Acquire opens path, takes a non-blocking ownership lock on it, and writes
// the current process's pid into it. It returns ErrPIDFileLocked when another
// supervisor already holds the lock.
func Acquire(path string) (*PIDFile, error) {
	return acquire(path, lockFile)
}

func acquire(path string, lock func(*os.File) error) (*PIDFile, error) {
	f, err := openPIDFile(path)
	if err != nil {
		return nil, err
	}
	if err := lock(f); err != nil {
		ownerPID, ownerKnown := readOwnerPID(f)
		f.Close()
		if lockUnavailable(err) {
			if ownerKnown {
				return nil, fmt.Errorf("%w: pid file %s is locked by process %d", ErrPIDFileLocked, path, ownerPID)
			}
			return nil, fmt.Errorf("%w: pid file %s (owner pid unavailable)", ErrPIDFileLocked, path)
		}
		return nil, fmt.Errorf("failed to lock pid file %s: %w", path, err)
	}
	if err := validatePIDFileLinkCount(f, path); err != nil {
		f.Close()
		return nil, err
	}
	if err := f.Truncate(0); err != nil {
		f.Close()
		return nil, err
	}
	if _, err := fmt.Fprintf(f, "%d\n", os.Getpid()); err != nil {
		f.Close()
		return nil, err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return nil, err
	}
	if err := finishPIDFileLock(f); err != nil {
		f.Close()
		return nil, fmt.Errorf("failed to finish pid file lock %s: %w", path, err)
	}
	return &PIDFile{file: f, path: path}, nil
}

func readOwnerPID(f *os.File) (int, bool) {
	var data [pidTextSize]byte
	n, err := readPIDText(f, data[:])
	if err != nil && err != io.EOF {
		return 0, false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data[:n])))
	return pid, err == nil && pid > 0
}

func (p *PIDFile) Close() error {
	if p == nil || p.file == nil {
		return nil
	}
	err := closePIDFile(p.file, p.path)
	p.file = nil
	return err
}

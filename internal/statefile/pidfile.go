package statefile

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

const pidTextSize = 64

// PIDFile is a pid file that has been acquired via Acquire. Closing it
// releases the lock and, if this process still owns the file on disk,
// removes it.
type PIDFile struct {
	file *os.File
	path string
}

// Acquire opens path, takes a non-blocking exclusive lock on it, and writes
// the current process's pid into it.
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
				return nil, fmt.Errorf("pid file %q is locked by process %d: %w", path, ownerPID, err)
			}
			return nil, fmt.Errorf("pid file %q is already locked (owner pid unavailable): %w", path, err)
		}
		return nil, fmt.Errorf("failed to lock pid file %q: %w", path, err)
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
	return &PIDFile{file: f, path: path}, nil
}

func readOwnerPID(f *os.File) (int, bool) {
	var data [pidTextSize]byte
	n, err := f.ReadAt(data[:], 0)
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
	if pathInfo, err := os.Stat(p.path); err == nil {
		if fileInfo, statErr := p.file.Stat(); statErr == nil && os.SameFile(pathInfo, fileInfo) {
			_ = os.Remove(p.path)
		}
	}
	err := p.file.Close()
	p.file = nil
	return err
}

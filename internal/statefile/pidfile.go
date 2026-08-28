package statefile

import (
	"fmt"
	"os"
)

// PIDFile is a pid file that has been acquired via Acquire. Closing it
// releases the lock and, if this process still owns the file on disk,
// removes it.
type PIDFile struct {
	file *os.File
	path string
}

// Acquire opens path, takes a blocking exclusive lock on it, and writes the
// current process's pid into it.
func Acquire(path string) (*PIDFile, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return nil, err
	}
	if err := lockFile(f); err != nil {
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

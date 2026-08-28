package starter

import (
	"fmt"
	"os"
	"syscall"
)

type pidFile struct {
	file *os.File
	path string
}

func acquirePIDFile(path string) (*pidFile, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
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
	return &pidFile{file: f, path: path}, nil
}

func (p *pidFile) Close() error {
	if p == nil || p.file == nil {
		return nil
	}
	// Do not remove a path that another process replaced while this process
	// was running. The lock makes the normal handoff safe.
	if pathInfo, err := os.Stat(p.path); err == nil {
		if fileInfo, statErr := p.file.Stat(); statErr == nil && os.SameFile(pathInfo, fileInfo) {
			_ = os.Remove(p.path)
		}
	}
	err := p.file.Close()
	p.file = nil
	return err
}

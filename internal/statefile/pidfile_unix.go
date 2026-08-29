//go:build !windows

package statefile

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"
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

func (p *runningPIDPath) open() (*os.File, error) {
	fd, err := unix.Openat(
		int(p.parent.Fd()),
		p.name,
		unix.O_RDWR|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC,
		0,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to open pid file %q: %w", p.path, err)
	}
	f := os.NewFile(uintptr(fd), p.path)
	if f == nil {
		_ = unix.Close(fd)
		return nil, fmt.Errorf("failed to open pid file %q", p.path)
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("failed to inspect pid file %q: %w", p.path, err)
	}
	if !info.Mode().IsRegular() {
		_ = f.Close()
		return nil, fmt.Errorf("pid file %q is not a regular file", p.path)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		_ = f.Close()
		return nil, fmt.Errorf("failed to inspect pid file %q metadata", p.path)
	}
	if stat.Nlink != 1 {
		_ = f.Close()
		return nil, fmt.Errorf("pid file %q has %d hard links, expected one", p.path, stat.Nlink)
	}
	return f, nil
}

func prepareRunningPIDPath(path string) (*runningPIDPath, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve pid file %q: %w", path, err)
	}
	parentPath := filepath.Dir(absPath)
	resolvedParent, err := filepath.EvalSymlinks(parentPath)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve pid file %q parent directory: %w", path, err)
	}
	parent, err := openDirectoryPath(resolvedParent)
	if err != nil {
		return nil, fmt.Errorf("failed to open pid file %q parent directory %q: %w", path, resolvedParent, err)
	}
	if err := validatePIDControlDirectory(parent, path, resolvedParent); err != nil {
		_ = parent.Close()
		return nil, err
	}
	return &runningPIDPath{path: path, parent: parent, name: filepath.Base(absPath)}, nil
}

func openDirectoryPath(path string) (*os.File, error) {
	current, err := os.OpenFile(string(os.PathSeparator), os.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	cleanPath := strings.TrimPrefix(filepath.Clean(path), string(os.PathSeparator))
	if cleanPath == "" {
		return current, nil
	}
	for _, name := range strings.Split(cleanPath, string(os.PathSeparator)) {
		fd, openErr := unix.Openat(
			int(current.Fd()),
			name,
			unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC,
			0,
		)
		if openErr != nil {
			_ = current.Close()
			return nil, openErr
		}
		next := os.NewFile(uintptr(fd), name)
		if next == nil {
			_ = unix.Close(fd)
			_ = current.Close()
			return nil, fmt.Errorf("failed to retain directory %q", name)
		}
		info, statErr := next.Stat()
		if statErr != nil {
			_ = next.Close()
			_ = current.Close()
			return nil, statErr
		}
		if !info.IsDir() {
			_ = next.Close()
			_ = current.Close()
			return nil, fmt.Errorf("path component %q is not a directory", name)
		}
		_ = current.Close()
		current = next
	}
	return current, nil
}

// validatePIDControlDirectory requires the control pathname to live in a
// directory that an unrelated uid cannot replace. User-owned mode-0700 runtime
// dirs are supported; world- or group-writable directories are rejected.
func validatePIDControlDirectory(parent *os.File, path, parentPath string) error {
	info, err := parent.Stat()
	if err != nil {
		return fmt.Errorf("failed to inspect pid file %q parent directory %q: %w", path, parentPath, err)
	}
	trustedUID := uint32(os.Geteuid())
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("failed to inspect owner of %q", parentPath)
	}
	if stat.Uid != 0 && stat.Uid != trustedUID {
		return fmt.Errorf("pid file %q has untrusted parent directory %q owned by uid %d", path, parentPath, stat.Uid)
	}
	if info.Mode().Perm()&0022 != 0 {
		return fmt.Errorf("pid file %q parent directory %q allows untrusted replacement", path, parentPath)
	}
	return nil
}

func (p *runningPIDPath) validate(f *os.File) error {
	openedInfo, err := f.Stat()
	if err != nil {
		return err
	}
	pathFile, err := p.open()
	if err != nil {
		return err
	}
	defer pathFile.Close()
	pathInfo, err := pathFile.Stat()
	if err != nil {
		return err
	}
	if !os.SameFile(openedInfo, pathInfo) {
		return fmt.Errorf("pid file %q was replaced while being validated", p.path)
	}
	return nil
}

func (p *runningPIDPath) close() error {
	return p.parent.Close()
}

func readPIDText(f *os.File, data []byte) (int, error) {
	return f.ReadAt(data, 0)
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

// TryLock attempts to take an exclusive, non-blocking lock on f. It is used
// by control.Stop to poll for the supervisor having exited: once the
// supervisor process dies, its lock on the pid file is released and this call
// starts succeeding. It returns syscall.EWOULDBLOCK while another process
// holds the lock; any other error means the lock check failed.
func TryLock(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
}

func processIsLive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

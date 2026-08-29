package statefile

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

const maxPIDFileSize = 64

// ErrPIDFileLocked means a live supervisor already holds the pid-file lock.
var ErrPIDFileLocked = errors.New("pid file is already locked")

// PIDFile is a pid file and its control lock acquired via Acquire. Closing it
// releases both locks and removes files this process still owns on disk.
type PIDFile struct {
	file        *os.File
	path        string
	controlFile *PIDFile
}

// RunningPID is a validated reference to a running supervisor. The pid is
// accepted only when it matches the process that owns both supervisor locks.
type RunningPID struct {
	file        *os.File
	controlFile *os.File
	pid         int
}

// Acquire opens path, takes non-blocking ownership locks on it and its control
// lock, and writes the current process's pid into both files. It returns
// ErrPIDFileLocked when another supervisor already holds either lock.
func Acquire(path string) (*PIDFile, error) {
	pidFile, err := acquire(path, func(f *os.File) error {
		return lockFile(f, path)
	})
	if err != nil {
		return nil, err
	}

	controlPath := controlLockPath(path)
	controlFile, err := acquire(controlPath, func(f *os.File) error {
		return lockFile(f, controlPath)
	})
	if err != nil {
		_ = pidFile.Close()
		return nil, err
	}
	pidFile.controlFile = controlFile
	return pidFile, nil
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
	var data [maxPIDFileSize]byte
	n, err := readPIDText(f, data[:])
	if err != nil && err != io.EOF {
		return 0, false
	}
	parsedPID, err := strconv.ParseInt(strings.TrimSpace(string(data[:n])), 10, 32)
	return int(parsedPID), err == nil && parsedPID > 0
}

func (p *PIDFile) Close() error {
	if p == nil || p.file == nil {
		return nil
	}
	err := closePIDFile(p.file, p.path)
	p.file = nil
	if p.controlFile != nil {
		err = errors.Join(err, p.controlFile.Close())
		p.controlFile = nil
	}
	return err
}

// OpenRunningPID opens path and verifies that its recorded pid owns the
// supervisor locks. Keeping the returned handles open also lets callers wait
// on the same files even if the pathnames are later replaced.
func OpenRunningPID(path string) (*RunningPID, error) {
	f, err := openRunningPIDFile(path)
	if err != nil {
		return nil, err
	}
	var controlFile *os.File

	closeWithError := func(err error) (*RunningPID, error) {
		_ = f.Close()
		if controlFile != nil {
			_ = controlFile.Close()
		}
		return nil, err
	}

	data, err := io.ReadAll(io.LimitReader(f, maxPIDFileSize+1))
	if err != nil {
		return closeWithError(err)
	}
	if len(data) > maxPIDFileSize {
		return closeWithError(fmt.Errorf("pid file %q is too large", path))
	}
	value := strings.TrimSpace(string(data))
	parsedPID, err := strconv.ParseInt(value, 10, 32)
	if err != nil || parsedPID <= 0 {
		return closeWithError(fmt.Errorf("invalid pid file %q", path))
	}
	pid := int(parsedPID)

	ownerPID, err := lockOwnerPID(f, path)
	if err != nil {
		return closeWithError(fmt.Errorf("failed to inspect pid file %q lock: %w", path, err))
	}
	if ownerPID == 0 {
		return closeWithError(fmt.Errorf("pid file %q is not locked by a running supervisor", path))
	}
	if ownerPID != pid {
		return closeWithError(fmt.Errorf("pid file %q records process %d, which does not match lock owner %d", path, pid, ownerPID))
	}

	controlPath := controlLockPath(path)
	controlFile, err = openRunningPIDFile(controlPath)
	if errors.Is(err, os.ErrNotExist) {
		// The sibling lock was added after the original pid-file protocol. Keep
		// accepting legacy supervisors that have no control lock.
		controlFile = nil
	} else if err != nil {
		return closeWithError(fmt.Errorf("failed to open control lock %q: %w", controlPath, err))
	} else {
		controlOwnerPID, err := lockOwnerPID(controlFile, controlPath)
		if err != nil {
			return closeWithError(fmt.Errorf("failed to inspect control lock %q: %w", controlPath, err))
		}
		if controlOwnerPID == 0 {
			return closeWithError(fmt.Errorf("control lock %q is not held by a running supervisor", controlPath))
		}
		if controlOwnerPID != pid {
			return closeWithError(fmt.Errorf("pid file %q records process %d, which does not match control lock owner %d", path, pid, controlOwnerPID))
		}
	}

	openedInfo, err := f.Stat()
	if err != nil {
		return closeWithError(err)
	}
	pathInfo, err := os.Lstat(path)
	if err != nil {
		return closeWithError(err)
	}
	if !pathInfo.Mode().IsRegular() || !os.SameFile(openedInfo, pathInfo) {
		return closeWithError(fmt.Errorf("pid file %q was replaced while being validated", path))
	}

	return &RunningPID{file: f, controlFile: controlFile, pid: pid}, nil
}

func controlLockPath(path string) string {
	return path + ".lock"
}

// ReadPID reads a pid only from a live supervisor-owned pid file.
func ReadPID(path string) (int, error) {
	running, err := OpenRunningPID(path)
	if err != nil {
		return 0, err
	}
	defer running.Close()
	return running.PID(), nil
}

// PID returns the validated supervisor process id.
func (p *RunningPID) PID() int {
	return p.pid
}

// Exited reports whether the supervisor has released its control lock.
func (p *RunningPID) Exited() (bool, error) {
	if p.controlFile != nil {
		return lockReleased(p.controlFile)
	}
	return lockReleased(p.file)
}

// Close releases the open pid-file reference.
func (p *RunningPID) Close() error {
	if p == nil || p.file == nil {
		return nil
	}
	err := p.file.Close()
	p.file = nil
	if p.controlFile != nil {
		err = errors.Join(err, p.controlFile.Close())
		p.controlFile = nil
	}
	return err
}

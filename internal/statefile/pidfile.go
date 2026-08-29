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

// PIDFile is a pid file acquired by Acquire. Closing it releases the lock and
// removes the pathname only when it still names the acquired inode.
type PIDFile struct {
	file *os.File
	path string
}

// RunningPID is a validated reference to a running supervisor. The returned
// file remains open so callers can wait on the same lock after validation.
type RunningPID struct {
	file *os.File
	pid  int
}

type runningPIDPath struct {
	path   string
	parent *os.File
	name   string
}

// Acquire opens path, takes the platform lifetime lock, and writes the
// current process's pid into it. The platform lock is deliberately the only
// persistent lock: adding a sibling lock creates a second replaceable name
// and breaks older pid-file users when stale residue is present.
func Acquire(path string) (*PIDFile, error) {
	f, err := acquire(path, func(f *os.File) error {
		return lockFile(f, path)
	})
	if err != nil {
		return nil, err
	}
	return f, nil
}

func acquire(path string, lock func(*os.File) error) (*PIDFile, error) {
	f, err := openPIDFile(path)
	if err != nil {
		return nil, err
	}
	if err := lock(f); err != nil {
		ownerPID, ownerKnown := readOwnerPID(f)
		_ = f.Close()
		if lockUnavailable(err) {
			if ownerKnown {
				return nil, fmt.Errorf("%w: pid file %s is locked by process %d", ErrPIDFileLocked, path, ownerPID)
			}
			return nil, fmt.Errorf("%w: pid file %s (owner pid unavailable)", ErrPIDFileLocked, path)
		}
		return nil, fmt.Errorf("failed to lock pid file %s: %w", path, err)
	}
	if err := validatePIDFileLinkCount(f, path); err != nil {
		_ = f.Close()
		return nil, err
	}
	if err := f.Truncate(0); err != nil {
		_ = f.Close()
		return nil, err
	}
	if _, err := fmt.Fprintf(f, "%d\n", os.Getpid()); err != nil {
		_ = f.Close()
		return nil, err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return nil, err
	}
	if err := finishPIDFileLock(f); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("failed to finish pid file lock %s: %w", path, err)
	}
	return &PIDFile{file: f, path: path}, nil
}

func readOwnerPID(f *os.File) (int, bool) {
	var data [pidTextSize]byte
	n, err := readPIDText(f, data[:])
	if err != nil && !errors.Is(err, io.EOF) {
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
	return err
}

// OpenRunningPID opens path without following a replacement symlink, checks
// the recorded pid against the live lock owner, and returns the retained file.
// Legacy flock-only files are accepted only when the lock is still held and
// the recorded process is live. Platforms without a kernel flock-owner query
// cannot provide stronger attribution for that old format.
func OpenRunningPID(path string) (*RunningPID, error) {
	preparedPath, err := prepareRunningPIDPath(path)
	if err != nil {
		return nil, err
	}
	defer preparedPath.close()

	f, err := preparedPath.open()
	if err != nil {
		return nil, err
	}
	closeWithError := func(err error) (*RunningPID, error) {
		_ = f.Close()
		return nil, err
	}

	data, err := io.ReadAll(io.LimitReader(f, pidTextSize+1))
	if err != nil {
		return closeWithError(err)
	}
	if len(data) > pidTextSize {
		return closeWithError(fmt.Errorf("pid file %q is too large", path))
	}
	parsedPID, err := strconv.ParseInt(strings.TrimSpace(string(data)), 10, 32)
	if err != nil || parsedPID <= 0 {
		return closeWithError(fmt.Errorf("invalid pid file %q", path))
	}
	pid := int(parsedPID)

	ownerPID, err := lockOwnerPID(f, path)
	if err != nil {
		return closeWithError(fmt.Errorf("failed to inspect pid file %q lock: %w", path, err))
	}
	if ownerPID > 0 {
		if ownerPID != pid {
			return closeWithError(fmt.Errorf("pid file %q records process %d, which does not match lock owner %d", path, pid, ownerPID))
		}
	} else {
		lockErr := TryLock(f)
		if lockErr == nil {
			return closeWithError(fmt.Errorf("pid file %q is not locked by a running supervisor", path))
		}
		if !lockUnavailable(lockErr) {
			return closeWithError(fmt.Errorf("failed to inspect legacy pid file %q lock: %w", path, lockErr))
		}
		if !processIsLive(pid) {
			return closeWithError(fmt.Errorf("pid file %q records process %d, which is not running", path, pid))
		}
	}

	if err := preparedPath.validate(f); err != nil {
		return closeWithError(err)
	}

	return &RunningPID{file: f, pid: pid}, nil
}

// PID returns the validated supervisor process id.
func (p *RunningPID) PID() int {
	return p.pid
}

// Exited reports whether the supervisor has released its lifetime lock.
func (p *RunningPID) Exited() (bool, error) {
	return lockReleased(p.file)
}

// Close releases the retained pid-file reference.
func (p *RunningPID) Close() error {
	if p == nil || p.file == nil {
		return nil
	}
	err := p.file.Close()
	p.file = nil
	return err
}

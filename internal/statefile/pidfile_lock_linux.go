//go:build linux

package statefile

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"
)

// Linux retains the flock lifetime protocol for old supervisors and overlays
// an inode-stable record lock so new control calls can identify the owner PID.
func lockFile(f *os.File, _ string) error {
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return err
	}
	lock := pidFileRecordLock()
	return syscall.FcntlFlock(f.Fd(), syscall.F_SETLK, &lock)
}

func lockOwnerPID(f *os.File, _ string) (int, error) {
	lock := pidFileRecordLock()
	if err := syscall.FcntlFlock(f.Fd(), syscall.F_GETLK, &lock); err != nil {
		return 0, err
	}
	if lock.Type != syscall.F_UNLCK {
		if lock.Pid <= 0 {
			return 0, fmt.Errorf("record lock has no process owner")
		}
		return int(lock.Pid), nil
	}

	flockPID, err := linuxFlockOwner(f)
	if err != nil {
		return 0, err
	}
	return flockPID, nil
}

func linuxFlockOwner(f *os.File) (int, error) {
	return linuxFlockOwnerAt(f, "/proc/locks")
}

func linuxFlockOwnerAt(f *os.File, locksPath string) (int, error) {
	info, err := f.Stat()
	if err != nil {
		return 0, err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, fmt.Errorf("pid file has unsupported stat data")
	}
	locks, err := os.Open(locksPath)
	if err != nil {
		return 0, nil
	}
	defer locks.Close()
	major := uint64(unix.Major(stat.Dev))
	minor := uint64(unix.Minor(stat.Dev))
	scanner := bufio.NewScanner(locks)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 8 || fields[1] != "FLOCK" || fields[3] != "WRITE" {
			continue
		}
		parts := strings.Split(fields[5], ":")
		if len(parts) != 3 {
			continue
		}
		lockMajor, majorErr := strconv.ParseUint(parts[0], 16, 64)
		lockMinor, minorErr := strconv.ParseUint(parts[1], 16, 64)
		lockInode, inodeErr := strconv.ParseUint(parts[2], 10, 64)
		if majorErr != nil || minorErr != nil || inodeErr != nil || lockMajor != major || lockMinor != minor || lockInode != stat.Ino {
			continue
		}
		pid, pidErr := strconv.Atoi(fields[4])
		if pidErr != nil || pid <= 0 {
			return 0, fmt.Errorf("invalid flock owner pid %q", fields[4])
		}
		return pid, nil
	}
	if err := scanner.Err(); err != nil {
		return 0, nil
	}
	return 0, nil
}

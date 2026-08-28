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

// inspectInodeLocks finds legacy BSD flock ownership and reports whether the
// inode also has a record lock. A record lock outside the expected path-bound
// byte means the file was locked under another pathname.
func inspectInodeLocks(f *os.File, _ int) (int, bool, error) {
	info, err := f.Stat()
	if err != nil {
		return 0, false, err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, false, fmt.Errorf("pid file has unsupported stat data")
	}

	locks, err := os.Open("/proc/locks")
	if err != nil {
		return 0, false, err
	}
	defer locks.Close()

	major := uint64(unix.Major(stat.Dev))
	minor := uint64(unix.Minor(stat.Dev))
	inode := stat.Ino
	flockPID := 0
	hasRecordLock := false
	scanner := bufio.NewScanner(locks)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 8 || fields[1] == "->" || fields[3] != "WRITE" {
			continue
		}
		lockMajor, lockMinor, lockInode, ok := parseProcLockIdentity(fields[5])
		if !ok || lockMajor != major || lockMinor != minor || lockInode != inode {
			continue
		}
		switch fields[1] {
		case "FLOCK":
			pid, parseErr := strconv.Atoi(fields[4])
			if parseErr != nil || pid <= 0 {
				return 0, false, fmt.Errorf("invalid flock owner pid %q", fields[4])
			}
			flockPID = pid
		case "POSIX":
			hasRecordLock = true
		}
	}
	if err := scanner.Err(); err != nil {
		return 0, false, err
	}
	return flockPID, hasRecordLock, nil
}

func parseProcLockIdentity(value string) (uint64, uint64, uint64, bool) {
	parts := strings.Split(value, ":")
	if len(parts) != 3 {
		return 0, 0, 0, false
	}
	major, err := strconv.ParseUint(parts[0], 16, 64)
	if err != nil {
		return 0, 0, 0, false
	}
	minor, err := strconv.ParseUint(parts[1], 16, 64)
	if err != nil {
		return 0, 0, 0, false
	}
	inode, err := strconv.ParseUint(parts[2], 10, 64)
	if err != nil {
		return 0, 0, 0, false
	}
	return major, minor, inode, true
}

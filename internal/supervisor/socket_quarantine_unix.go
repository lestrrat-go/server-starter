//go:build linux || darwin

package supervisor

import (
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

const (
	quarantineDirPrefix = ".server-starter-socket-"
	quarantineDirName   = quarantineDirPrefix + "quarantine"
	quarantineEntryName = "socket"
)

type unixSocketQuarantine struct {
	parentFD   int
	dirFD      int
	parentPath string
	dirName    string
	entryName  string
	dirStat    unix.Stat_t
}

func newSocketQuarantine(path string, hooks socketCleanupHooks) (socketQuarantine, error) {
	parentPath, entryName := filepath.Split(path)
	if parentPath == "" {
		parentPath = "." + string(filepath.Separator)
	}
	parentFD, err := openQuarantineDirectory(parentPath)
	if err != nil {
		return nil, err
	}

	created, err := ensureQuarantineDir(parentFD)
	if err != nil {
		_ = unix.Close(parentFD)
		return nil, err
	}
	dirPath := parentPath + quarantineDirName
	if created && hooks.afterQuarantineMkdir != nil {
		hooks.afterQuarantineMkdir(dirPath)
	}
	dirFD, err := openQuarantineDirectoryAt(parentFD, quarantineDirName)
	if err != nil {
		if hooks.afterQuarantineOpenFailure != nil {
			hooks.afterQuarantineOpenFailure(dirPath)
		}
		_ = unix.Close(parentFD)
		return nil, err
	}

	quarantine := &unixSocketQuarantine{
		parentFD:   parentFD,
		dirFD:      dirFD,
		parentPath: parentPath,
		dirName:    quarantineDirName,
		entryName:  entryName,
	}
	if err := unix.Fstat(dirFD, &quarantine.dirStat); err != nil {
		quarantine.close()
		return nil, err
	}
	if err := validateQuarantineDirectory(&quarantine.dirStat); err != nil {
		quarantine.close()
		return nil, err
	}
	return quarantine, nil
}

func ensureQuarantineDir(parentFD int) (bool, error) {
	if err := unix.Mkdirat(parentFD, quarantineDirName, 0o700); err != nil {
		if err == unix.EEXIST {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func validateQuarantineDirectory(stat *unix.Stat_t) error {
	if stat.Mode&unix.S_IFMT != unix.S_IFDIR {
		return fmt.Errorf("quarantine path is not a directory")
	}
	if stat.Uid != uint32(os.Geteuid()) {
		return fmt.Errorf("quarantine directory owner %d does not match effective user %d", stat.Uid, os.Geteuid())
	}
	permissions := stat.Mode & 0o777
	if permissions&0o077 != 0 || permissions&0o300 != 0o300 {
		return fmt.Errorf("quarantine directory permissions %#o are not private and write-search capable", permissions)
	}
	return nil
}

func (q *unixSocketQuarantine) moveIn() error {
	return renameSocketEntryNoReplace(q.parentFD, q.entryName, q.dirFD, quarantineEntryName)
}

func (q *unixSocketQuarantine) entryIsSocket() (bool, error) {
	var stat unix.Stat_t
	if err := unix.Fstatat(q.dirFD, quarantineEntryName, &stat, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return false, err
	}
	return stat.Mode&unix.S_IFMT == unix.S_IFSOCK, nil
}

func (q *unixSocketQuarantine) restore() error {
	return renameSocketEntryNoReplace(q.dirFD, quarantineEntryName, q.parentFD, q.entryName)
}

func (q *unixSocketQuarantine) removeEntry() error {
	return unix.Unlinkat(q.dirFD, quarantineEntryName, 0)
}

func (q *unixSocketQuarantine) cleanup() error {
	var current unix.Stat_t
	if err := unix.Fstatat(q.parentFD, q.dirName, &current, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return err
	}
	if current.Dev != q.dirStat.Dev || current.Ino != q.dirStat.Ino || current.Mode&unix.S_IFMT != unix.S_IFDIR {
		return fmt.Errorf("quarantine directory changed; original directory retained through its open handle")
	}
	// Removing the directory by name would resolve it again after the identity
	// check and could remove a replacement. Retain the empty directory instead.
	return nil
}

func (q *unixSocketQuarantine) close() {
	_ = unix.Close(q.dirFD)
	_ = unix.Close(q.parentFD)
}

func (q *unixSocketQuarantine) location() string {
	return q.parentPath + q.dirName + string(filepath.Separator) + quarantineEntryName
}

func safeSocketQuarantineAvailable() bool {
	return true
}

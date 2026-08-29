//go:build linux || darwin

package supervisor

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"path/filepath"

	"golang.org/x/sys/unix"
)

const quarantineEntryName = "socket"

type unixSocketQuarantine struct {
	parentFD   int
	dirFD      int
	parentPath string
	dirName    string
	entryName  string
	dirStat    unix.Stat_t
}

func newSocketQuarantine(path string) (socketQuarantine, error) {
	parentPath := filepath.Dir(path)
	parentFD, err := unix.Open(parentPath, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}

	dirName, err := makeQuarantineDir(parentFD)
	if err != nil {
		_ = unix.Close(parentFD)
		return nil, err
	}
	dirFD, err := unix.Openat(parentFD, dirName, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		_ = unix.Unlinkat(parentFD, dirName, unix.AT_REMOVEDIR)
		_ = unix.Close(parentFD)
		return nil, err
	}

	quarantine := &unixSocketQuarantine{
		parentFD:   parentFD,
		dirFD:      dirFD,
		parentPath: parentPath,
		dirName:    dirName,
		entryName:  filepath.Base(path),
	}
	if err := unix.Fstat(dirFD, &quarantine.dirStat); err != nil {
		quarantine.close()
		return nil, err
	}
	return quarantine, nil
}

func makeQuarantineDir(parentFD int) (string, error) {
	for range 8 {
		var random [16]byte
		if _, err := rand.Read(random[:]); err != nil {
			return "", err
		}
		name := ".server-starter-socket-" + hex.EncodeToString(random[:])
		if err := unix.Mkdirat(parentFD, name, 0o700); err == nil {
			return name, nil
		} else if err != unix.EEXIST {
			return "", err
		}
	}
	return "", fmt.Errorf("allocate unique quarantine directory")
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
	return unix.Unlinkat(q.parentFD, q.dirName, unix.AT_REMOVEDIR)
}

func (q *unixSocketQuarantine) close() {
	_ = unix.Close(q.dirFD)
	_ = unix.Close(q.parentFD)
}

func (q *unixSocketQuarantine) location() string {
	return filepath.Join(q.parentPath, q.dirName, quarantineEntryName)
}

func safeSocketQuarantineAvailable() bool {
	return true
}

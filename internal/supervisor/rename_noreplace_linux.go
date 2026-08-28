//go:build linux

package supervisor

import (
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

func renameNoReplaceAt(oldDir *os.File, oldName string, newDir *os.File, newName string) error {
	return unix.Renameat2(
		int(oldDir.Fd()),
		oldName,
		int(newDir.Fd()),
		newName,
		unix.RENAME_NOREPLACE,
	)
}

func pathIsSocketAt(dir *os.File, name string) (bool, error) {
	var stat unix.Stat_t
	if err := unix.Fstatat(int(dir.Fd()), name, &stat, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return false, err
	}
	return stat.Mode&unix.S_IFMT == unix.S_IFSOCK, nil
}

func removeAt(dir *os.File, name string) error {
	return unix.Unlinkat(int(dir.Fd()), name, 0)
}

func createPrivateDirAt(dir *os.File, name string) (*os.File, error) {
	dirFD := int(dir.Fd())
	if err := unix.Mkdirat(dirFD, name, 0700); err != nil {
		return nil, err
	}

	fd, err := unix.Openat(
		dirFD,
		name,
		unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW,
		0,
	)
	if err != nil {
		_ = unix.Unlinkat(dirFD, name, unix.AT_REMOVEDIR)
		return nil, err
	}
	return os.NewFile(uintptr(fd), filepath.Join(dir.Name(), name)), nil
}

func removeDirAt(dir *os.File, name string) error {
	return unix.Unlinkat(int(dir.Fd()), name, unix.AT_REMOVEDIR)
}

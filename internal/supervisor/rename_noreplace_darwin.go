//go:build darwin

package supervisor

import (
	"os"

	"golang.org/x/sys/unix"
)

func renameNoReplaceAt(dir *os.File, oldName, newName string) error {
	fd := int(dir.Fd())
	return unix.RenameatxNp(fd, oldName, fd, newName, unix.RENAME_EXCL)
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

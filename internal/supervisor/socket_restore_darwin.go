//go:build darwin

package supervisor

import "golang.org/x/sys/unix"

func restoreSocketEntry(oldpath, newpath string) error {
	return unix.RenameatxNp(unix.AT_FDCWD, oldpath, unix.AT_FDCWD, newpath, unix.RENAME_EXCL)
}

//go:build linux

package supervisor

import "golang.org/x/sys/unix"

func restoreSocketEntry(oldpath, newpath string) error {
	return unix.Renameat2(unix.AT_FDCWD, oldpath, unix.AT_FDCWD, newpath, unix.RENAME_NOREPLACE)
}

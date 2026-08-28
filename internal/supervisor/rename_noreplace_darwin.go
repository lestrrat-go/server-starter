//go:build darwin

package supervisor

import "golang.org/x/sys/unix"

func renameNoReplace(oldpath, newpath string) error {
	return unix.RenamexNp(oldpath, newpath, unix.RENAME_EXCL)
}

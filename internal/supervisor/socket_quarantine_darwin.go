//go:build darwin

package supervisor

import "golang.org/x/sys/unix"

func renameSocketEntryNoReplace(oldDirFD int, oldName string, newDirFD int, newName string) error {
	return unix.RenameatxNp(oldDirFD, oldName, newDirFD, newName, unix.RENAME_EXCL)
}

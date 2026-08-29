//go:build linux

package supervisor

import "golang.org/x/sys/unix"

func renameSocketEntryNoReplace(oldDirFD int, oldName string, newDirFD int, newName string) error {
	return unix.Renameat2(oldDirFD, oldName, newDirFD, newName, unix.RENAME_NOREPLACE)
}

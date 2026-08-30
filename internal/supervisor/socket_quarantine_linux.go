//go:build linux

package supervisor

import (
	"strconv"

	"golang.org/x/sys/unix"
)

func renameSocketEntryNoReplace(oldDirFD int, oldName string, newDirFD int, newName string) error {
	return unix.Renameat2(oldDirFD, oldName, newDirFD, newName, unix.RENAME_NOREPLACE)
}

func linkSocketFDNoReplace(sourceFD int, destinationDirFD int, destinationName string) error {
	return unix.Linkat(
		unix.AT_FDCWD,
		"/proc/self/fd/"+strconv.Itoa(sourceFD),
		destinationDirFD,
		destinationName,
		unix.AT_SYMLINK_FOLLOW,
	)
}

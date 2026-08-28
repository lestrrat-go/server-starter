//go:build linux

package supervisor

import (
	"os"

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

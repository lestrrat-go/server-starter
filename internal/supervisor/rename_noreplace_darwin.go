//go:build darwin

package supervisor

import (
	"os"

	"golang.org/x/sys/unix"
)

func renameNoReplaceAt(oldDir *os.File, oldName string, newDir *os.File, newName string) error {
	return unix.RenameatxNp(
		int(oldDir.Fd()),
		oldName,
		int(newDir.Fd()),
		newName,
		unix.RENAME_EXCL,
	)
}

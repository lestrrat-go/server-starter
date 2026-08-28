//go:build aix || dragonfly || freebsd || netbsd || openbsd || solaris

package supervisor

import "os"

func renameNoReplaceAt(oldDir *os.File, oldName string, newDir *os.File, newName string) error {
	return renameNoReplaceByLinkAt(oldDir, oldName, newDir, newName)
}

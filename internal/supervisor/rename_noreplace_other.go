//go:build !linux && !darwin && !windows

package supervisor

import (
	"os"
	"path/filepath"
)

func renameNoReplaceAt(dir *os.File, oldName, newName string) error {
	oldpath := filepath.Join(dir.Name(), oldName)
	newpath := filepath.Join(dir.Name(), newName)
	if err := os.Link(oldpath, newpath); err != nil {
		return err
	}
	return os.Remove(oldpath)
}

func pathIsSocketAt(dir *os.File, name string) (bool, error) {
	info, err := os.Lstat(filepath.Join(dir.Name(), name))
	if err != nil {
		return false, err
	}
	return info.Mode()&os.ModeSocket != 0, nil
}

func removeAt(dir *os.File, name string) error {
	return os.Remove(filepath.Join(dir.Name(), name))
}

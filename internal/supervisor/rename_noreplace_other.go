//go:build !linux && !darwin && !windows

package supervisor

import (
	"os"
	"path/filepath"
)

func renameNoReplaceAt(oldDir *os.File, oldName string, _ *os.File, newName string) error {
	return unsupportedRenameNoReplaceAt(oldDir, oldName, newName)
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

func createPrivateDirAt(dir *os.File, name string) (*os.File, error) {
	path := filepath.Join(dir.Name(), name)
	if err := os.Mkdir(path, 0700); err != nil {
		return nil, err
	}
	privateDir, err := os.Open(path)
	if err != nil {
		_ = os.Remove(path)
		return nil, err
	}
	return privateDir, nil
}

func removeDirAt(dir *os.File, name string) error {
	return os.Remove(filepath.Join(dir.Name(), name))
}

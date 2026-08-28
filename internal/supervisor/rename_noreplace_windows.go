//go:build windows

package supervisor

import (
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
)

func renameNoReplaceAt(oldDir *os.File, oldName string, newDir *os.File, newName string) error {
	oldpath := filepath.Join(oldDir.Name(), oldName)
	newpath := filepath.Join(newDir.Name(), newName)
	oldpathPtr, err := windows.UTF16PtrFromString(oldpath)
	if err != nil {
		return err
	}
	newpathPtr, err := windows.UTF16PtrFromString(newpath)
	if err != nil {
		return err
	}
	return windows.MoveFileEx(oldpathPtr, newpathPtr, 0)
}

func moveToQuarantineAt(oldDir *os.File, oldName string, newDir *os.File, newName string) error {
	return renameNoReplaceAt(oldDir, oldName, newDir, newName)
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
	// os.Open omits FILE_SHARE_DELETE. Keeping this handle open prevents the
	// quarantine pathname from being renamed or replaced before cleanup.
	return privateDir, nil
}

func removeDirAt(dir *os.File, name string) error {
	return os.Remove(filepath.Join(dir.Name(), name))
}

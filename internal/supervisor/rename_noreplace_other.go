//go:build !aix && !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd && !solaris && !windows

package supervisor

import (
	"fmt"
	"os"
	"path/filepath"
)

type pathIdentity struct {
	info os.FileInfo
}

func pathIdentityAt(dir *os.File, name string) (pathIdentity, error) {
	info, err := os.Lstat(filepath.Join(dir.Name(), name))
	if err != nil {
		return pathIdentity{}, err
	}
	return pathIdentity{info: info}, nil
}

func pathIdentityForFile(file *os.File) (pathIdentity, error) {
	info, err := file.Stat()
	if err != nil {
		return pathIdentity{}, err
	}
	return pathIdentity{info: info}, nil
}

func samePathIdentity(left, right pathIdentity) bool {
	return os.SameFile(left.info, right.info) && left.info.Mode().Type() == right.info.Mode().Type()
}

func (identity pathIdentity) isSocket() bool {
	return identity.info.Mode()&os.ModeSocket != 0
}

func renameNoReplaceEntryAt(
	oldDir *os.File,
	oldName string,
	newDir *os.File,
	newName string,
	expected pathIdentity,
) error {
	current, err := pathIdentityAt(oldDir, oldName)
	if err != nil {
		return err
	}
	if !samePathIdentity(current, expected) {
		return fmt.Errorf("filesystem entry changed before restoration")
	}
	return renameNoReplaceAt(oldDir, oldName, newDir, newName)
}

func renameNoReplaceAt(oldDir *os.File, oldName string, _ *os.File, newName string) error {
	return unsupportedRenameNoReplaceAt(oldDir, oldName, newName)
}

func moveToQuarantineAt(oldDir *os.File, oldName string, _ *os.File, newName string) error {
	return unsupportedRenameNoReplaceAt(oldDir, oldName, newName)
}

func removeAt(dir *os.File, name string, expected pathIdentity) error {
	current, err := pathIdentityAt(dir, name)
	if err != nil {
		return err
	}
	if !samePathIdentity(current, expected) {
		return fmt.Errorf("filesystem entry changed before removal")
	}
	return os.Remove(filepath.Join(dir.Name(), name))
}

func createPrivateDirAt(dir *os.File, name string) (*os.File, error) {
	path := filepath.Join(dir.Name(), name)
	if err := os.Mkdir(path, 0700); err != nil {
		return nil, err
	}
	created, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	privateDir, err := os.Open(path)
	if err != nil {
		current, statErr := os.Lstat(path)
		if statErr == nil && os.SameFile(created, current) {
			_ = os.Remove(path)
		}
		return nil, err
	}
	opened, err := privateDir.Stat()
	if err != nil {
		_ = privateDir.Close()
		return nil, err
	}
	if !os.SameFile(created, opened) {
		_ = privateDir.Close()
		return nil, fmt.Errorf("quarantine directory changed between creation and open")
	}
	return privateDir, nil
}

func removeDirAt(dir *os.File, name string, expected pathIdentity) error {
	current, err := pathIdentityAt(dir, name)
	if err != nil {
		return err
	}
	if !samePathIdentity(current, expected) {
		return fmt.Errorf("quarantine directory changed before removal")
	}
	return os.Remove(filepath.Join(dir.Name(), name))
}

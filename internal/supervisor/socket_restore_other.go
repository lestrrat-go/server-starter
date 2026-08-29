//go:build !darwin && !linux && !windows

package supervisor

import (
	"fmt"
	"os"
)

func restoreSocketEntry(oldpath, newpath string) error {
	info, err := os.Lstat(oldpath)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return fmt.Errorf("atomic no-replace restoration is unavailable for directories")
	}
	if err := os.Link(oldpath, newpath); err != nil {
		return err
	}
	restored, err := os.Lstat(newpath)
	if err != nil {
		return err
	}
	if !os.SameFile(info, restored) {
		return fmt.Errorf("restoration destination changed")
	}
	return os.Remove(oldpath)
}

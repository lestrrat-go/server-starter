//go:build !linux && !darwin && !windows

package supervisor

import "os"

func renameNoReplace(oldpath, newpath string) error {
	if err := os.Link(oldpath, newpath); err != nil {
		return err
	}
	return os.Remove(oldpath)
}

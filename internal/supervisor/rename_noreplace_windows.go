//go:build windows

package supervisor

import "golang.org/x/sys/windows"

func renameNoReplace(oldpath, newpath string) error {
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

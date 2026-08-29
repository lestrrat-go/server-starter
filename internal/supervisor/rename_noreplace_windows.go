//go:build windows

package supervisor

import (
	"fmt"
	"os"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"
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

func pinSocketAt(oldDir *os.File, oldName string, _ *os.File, _ string) (pathIdentity, error) {
	return pathIdentityAt(oldDir, oldName)
}

func unpinSocketAt(_ *os.File, _ string, _ pathIdentity) error {
	return nil
}

type fileRenameInformation struct {
	replaceIfExists uint32
	rootDirectory   windows.Handle
	fileNameLength  uint32
	fileName        [1]uint16
}

func renameNoReplaceEntryAt(
	oldDir *os.File,
	oldName string,
	newDir *os.File,
	newName string,
	expected pathIdentity,
) error {
	oldPath := filepath.Join(oldDir.Name(), oldName)
	oldPathPtr, err := windows.UTF16PtrFromString(oldPath)
	if err != nil {
		return err
	}
	handle, err := windows.CreateFile(
		oldPathPtr,
		windows.DELETE|windows.FILE_READ_ATTRIBUTES,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT,
		0,
	)
	if err != nil {
		return err
	}
	file := os.NewFile(uintptr(handle), oldPath)
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	if !samePathIdentity(pathIdentity{info: info}, expected) {
		return fmt.Errorf("filesystem entry changed before restoration")
	}

	newNameUTF16, err := windows.UTF16FromString(newName)
	if err != nil {
		return err
	}
	fileNameLength := len(newNameUTF16)*2 - 2
	var renameInfo fileRenameInformation
	bufferSize := int(unsafe.Offsetof(renameInfo.fileName)) + fileNameLength
	buffer := make([]byte, bufferSize)
	typedBuffer := (*fileRenameInformation)(unsafe.Pointer(&buffer[0]))
	typedBuffer.rootDirectory = windows.Handle(newDir.Fd())
	typedBuffer.fileNameLength = uint32(fileNameLength)
	copy(
		(*[windows.MAX_LONG_PATH]uint16)(unsafe.Pointer(&typedBuffer.fileName[0]))[:fileNameLength/2:fileNameLength/2],
		newNameUTF16,
	)
	var status windows.IO_STATUS_BLOCK
	return windows.NtSetInformationFile(
		handle,
		&status,
		&buffer[0],
		uint32(bufferSize),
		windows.FileRenameInformation,
	)
}

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

func removeAt(dir *os.File, name string, expected pathIdentity) error {
	return removeExpectedPath(filepath.Join(dir.Name(), name), expected)
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
	pathPtr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	handle, err := windows.CreateFile(
		pathPtr,
		windows.GENERIC_READ,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT,
		0,
	)
	if err != nil {
		_ = removeExpectedPath(path, pathIdentity{info: created})
		return nil, err
	}
	privateDir := os.NewFile(uintptr(handle), path)
	opened, err := privateDir.Stat()
	if err != nil {
		_ = privateDir.Close()
		_ = removeExpectedPath(path, pathIdentity{info: created})
		return nil, err
	}
	if !os.SameFile(created, opened) {
		_ = privateDir.Close()
		return nil, fmt.Errorf("quarantine directory changed between creation and open")
	}
	// Omitting FILE_SHARE_DELETE keeps the verified quarantine name tied to
	// this directory until cleanup starts.
	return privateDir, nil
}

func removeDirAt(dir *os.File, name string, expected pathIdentity) error {
	return removeExpectedPath(filepath.Join(dir.Name(), name), expected)
}

func closeAndRemoveSocketQuarantine(parent, quarantine *os.File, name string) error {
	identity, err := pathIdentityForFile(quarantine)
	if err != nil {
		return err
	}
	if err := quarantine.Close(); err != nil {
		return err
	}
	return removeDirAt(parent, name, identity)
}

func removeExpectedPath(path string, expected pathIdentity) error {
	pathPtr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	handle, err := windows.CreateFile(
		pathPtr,
		windows.DELETE|windows.FILE_READ_ATTRIBUTES,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT,
		0,
	)
	if err != nil {
		return err
	}
	file := os.NewFile(uintptr(handle), path)
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	if !samePathIdentity(pathIdentity{info: info}, expected) {
		return fmt.Errorf("filesystem entry changed before removal")
	}
	deleteFile := byte(1)
	return windows.SetFileInformationByHandle(
		handle,
		windows.FileDispositionInfo,
		&deleteFile,
		uint32(1),
	)
}

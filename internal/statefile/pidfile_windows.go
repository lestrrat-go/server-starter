package statefile

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

func openPIDFile(path string) (*os.File, error) {
	pathp, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open pid file %q: %w", path, err)
	}

	handle, err := createPIDFile(pathp, windows.CREATE_NEW)
	if errors.Is(err, windows.ERROR_FILE_EXISTS) || errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		handle, err = createPIDFile(pathp, windows.OPEN_EXISTING)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to open pid file %q: %w", path, err)
	}

	f := os.NewFile(uintptr(handle), path)
	if f == nil {
		windows.CloseHandle(handle)
		return nil, fmt.Errorf("failed to open pid file %q", path)
	}
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("failed to inspect pid file %q: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		f.Close()
		return nil, fmt.Errorf("pid file %q is not a regular file", path)
	}

	var handleInfo windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &handleInfo); err != nil {
		f.Close()
		return nil, fmt.Errorf("failed to inspect pid file %q: %w", path, err)
	}
	if handleInfo.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		f.Close()
		return nil, fmt.Errorf("pid file %q is a reparse point", path)
	}
	if handleInfo.NumberOfLinks != 1 {
		f.Close()
		return nil, fmt.Errorf("pid file %q has %d hard links, expected one", path, handleInfo.NumberOfLinks)
	}

	return f, nil
}

func createPIDFile(path *uint16, disposition uint32) (windows.Handle, error) {
	return windows.CreateFile(
		path,
		windows.GENERIC_READ|windows.GENERIC_WRITE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE,
		nil,
		disposition,
		windows.FILE_ATTRIBUTE_NORMAL|windows.FILE_FLAG_OPEN_REPARSE_POINT,
		0,
	)
}

func lockFile(f *os.File) error {
	// Keep the lock outside the PID text so a contender can read the owner
	// through a second handle while this exclusive lock is held.
	overlapped := windows.Overlapped{Offset: pidTextSize}
	return windows.LockFileEx(
		windows.Handle(f.Fd()),
		windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY,
		0,
		1,
		0,
		&overlapped,
	)
}

func validatePIDFileLinkCount(f *os.File, path string) error {
	var handleInfo windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(windows.Handle(f.Fd()), &handleInfo); err != nil {
		return fmt.Errorf("failed to inspect pid file %q: %w", path, err)
	}
	if handleInfo.NumberOfLinks != 1 {
		return fmt.Errorf("pid file %q has %d hard links, expected one", path, handleInfo.NumberOfLinks)
	}
	return nil
}

func lockUnavailable(err error) bool {
	return errors.Is(err, windows.ERROR_LOCK_VIOLATION)
}

// TryLock is used by control.Stop to poll for the supervisor having
// exited. --stop itself is unsupported on Windows (see signal_windows.go),
// so this is unreachable in practice; it exists to keep the platform seam
// symmetric and to fail loudly rather than silently if that ever changes.
func TryLock(f *os.File) error {
	return fmt.Errorf("waiting for a stopped process is not supported on windows")
}

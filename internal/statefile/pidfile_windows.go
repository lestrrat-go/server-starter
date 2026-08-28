package statefile

import (
	"errors"
	"fmt"
	"io"
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

func readPIDText(f *os.File, data []byte) (int, error) {
	n, err := f.ReadAt(data, 0)
	if !errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
		return n, err
	}

	info, err := f.Stat()
	if err != nil {
		return 0, err
	}
	size := min(int64(len(data)), info.Size())
	if size == 0 {
		return 0, io.EOF
	}

	// Windows byte-range locks do not apply to mapped views. A read-only
	// mapping can therefore inspect PID text protected by a legacy lock.
	mapping, err := windows.CreateFileMapping(
		windows.Handle(f.Fd()),
		nil,
		windows.PAGE_READONLY,
		0,
		0,
		nil,
	)
	if err != nil {
		return 0, err
	}
	defer windows.CloseHandle(mapping)

	address, err := windows.MapViewOfFile(mapping, windows.FILE_MAP_READ, 0, 0, uintptr(size))
	if err != nil {
		return 0, err
	}
	defer windows.UnmapViewOfFile(address)

	var bytesRead uintptr
	if err := windows.ReadProcessMemory(
		windows.CurrentProcess(),
		address,
		&data[0],
		uintptr(size),
		&bytesRead,
	); err != nil {
		return 0, err
	}
	return int(bytesRead), nil
}

func lockFile(f *os.File, _ string) error {
	// Start with the legacy exclusive byte-zero lock so old and current
	// supervisors cannot both acquire the PID file.
	var overlapped windows.Overlapped
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

func closePIDFile(f *os.File, path string) error {
	owned := false
	if pathInfo, err := os.Stat(path); err == nil {
		if fileInfo, statErr := f.Stat(); statErr == nil {
			owned = os.SameFile(pathInfo, fileInfo)
		}
	}
	closeErr := f.Close()
	var removeErr error
	if owned {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			removeErr = err
		}
	}
	return errors.Join(removeErr, closeErr)
}

func finishPIDFileLock(f *os.File) error {
	// Overlay a shared lock on the same handle, then release the exclusive
	// lock. The shared lock still rejects every contender's exclusive lock,
	// including the legacy lock, while allowing the PID text to be read.
	var overlapped windows.Overlapped
	if err := windows.LockFileEx(
		windows.Handle(f.Fd()),
		windows.LOCKFILE_FAIL_IMMEDIATELY,
		0,
		1,
		0,
		&overlapped,
	); err != nil {
		return err
	}
	return windows.UnlockFileEx(windows.Handle(f.Fd()), 0, 1, 0, &overlapped)
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

func lockOwnerPID(f *os.File, _ string, _ int) (int, pidLockKind, error) {
	return 0, pidLockUnknown, fmt.Errorf("inspecting pid-file lock ownership is not supported on windows")
}

func lockReleased(f *os.File, _ string, _ pidLockKind) (bool, error) {
	return false, fmt.Errorf("waiting for a stopped process is not supported on windows")
}

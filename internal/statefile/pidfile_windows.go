package statefile

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

func lockFile(f *os.File) error {
	// The file is truncated and rewritten after the lock is taken, so its
	// length at lock time may not cover the eventual content. Lock a
	// one-byte range instead of the whole file so the lock stays valid
	// regardless of how the content shrinks or grows afterward.
	var overlapped windows.Overlapped
	return windows.LockFileEx(
		windows.Handle(f.Fd()),
		windows.LOCKFILE_EXCLUSIVE_LOCK,
		0,
		1,
		0,
		&overlapped,
	)
}

// TryLock is used by control.Stop to poll for the supervisor having
// exited. --stop itself is unsupported on Windows (see signal_windows.go),
// so this is unreachable in practice; it exists to keep the platform seam
// symmetric and to fail loudly rather than silently if that ever changes.
func TryLock(f *os.File) error {
	return fmt.Errorf("waiting for a stopped process is not supported on windows")
}

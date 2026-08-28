package starter

import (
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

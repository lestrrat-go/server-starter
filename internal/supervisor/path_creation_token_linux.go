//go:build linux

package supervisor

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

func creationTokenAt(dir *os.File, name string, _ *unix.Stat_t) string {
	if token := fileHandleToken(int(dir.Fd()), name, 0); token != "" {
		return token
	}
	return statxBirthToken(int(dir.Fd()), name, unix.AT_SYMLINK_NOFOLLOW)
}

func creationTokenForFile(file *os.File, _ *unix.Stat_t) string {
	if token := fileHandleToken(int(file.Fd()), "", unix.AT_EMPTY_PATH); token != "" {
		return token
	}
	return statxBirthToken(int(file.Fd()), "", unix.AT_EMPTY_PATH|unix.AT_SYMLINK_NOFOLLOW)
}

func fileHandleToken(dirFD int, name string, flags int) string {
	handle, mountID, err := unix.NameToHandleAt(dirFD, name, flags)
	if err != nil {
		return ""
	}
	return fmt.Sprintf("handle:%d:%d:%x", mountID, handle.Type(), handle.Bytes())
}

func statxBirthToken(dirFD int, name string, flags int) string {
	var stat unix.Statx_t
	if err := unix.Statx(dirFD, name, flags, unix.STATX_BTIME, &stat); err != nil {
		return ""
	}
	if stat.Mask&unix.STATX_BTIME == 0 {
		return ""
	}
	return fmt.Sprintf("birth:%d:%d", stat.Btime.Sec, stat.Btime.Nsec)
}

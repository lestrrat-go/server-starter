//go:build !linux && !windows

package statefile

import "golang.org/x/sys/unix"

const directoryAccessFlag = unix.O_RDONLY

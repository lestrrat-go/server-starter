//go:build linux

package statefile

import "golang.org/x/sys/unix"

const directoryAccessFlag = unix.O_PATH

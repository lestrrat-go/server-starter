//go:build freebsd || solaris

package statefile

import "golang.org/x/sys/unix"

const directoryAccessFlag = unix.O_SEARCH
const useRootAnchoredDirectoryPath = false

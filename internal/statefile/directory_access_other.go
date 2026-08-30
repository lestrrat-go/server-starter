//go:build !linux && !windows && !darwin && !freebsd && !solaris

package statefile

import "golang.org/x/sys/unix"

const directoryAccessFlag = unix.O_RDONLY
const useRootAnchoredDirectoryPath = true

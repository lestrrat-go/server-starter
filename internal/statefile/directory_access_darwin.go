//go:build darwin

package statefile

import "golang.org/x/sys/unix"

const directoryAccessFlag = unix.O_EVTONLY
const useRootAnchoredDirectoryPath = false

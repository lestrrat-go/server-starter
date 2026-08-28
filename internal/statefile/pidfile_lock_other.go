//go:build !linux && !windows

package statefile

import "os"

func inspectInodeLocks(_ *os.File) (int, bool, error) {
	return 0, false, nil
}

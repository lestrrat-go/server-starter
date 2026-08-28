//go:build !darwin && !linux && !windows

package statefile

import "os"

func inspectInodeLocks(_ *os.File, _ int) (int, bool, error) {
	return 0, false, nil
}

//go:build !darwin && !linux

package supervisor

func newSocketQuarantine(string) (socketQuarantine, error) {
	return nil, errSafeSocketCleanupUnavailable
}

func safeSocketQuarantineAvailable() bool {
	return false
}

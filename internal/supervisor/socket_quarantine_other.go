//go:build !darwin && !linux

package supervisor

func newSocketQuarantine(string, socketCleanupHooks) (socketQuarantine, error) {
	return nil, errSafeSocketCleanupUnavailable
}

func safeSocketQuarantineAvailable() bool {
	return false
}

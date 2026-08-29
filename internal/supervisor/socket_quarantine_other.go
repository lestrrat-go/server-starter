//go:build !darwin && !linux

package supervisor

func newSocketQuarantine(string, map[string]struct{}, socketCleanupHooks) (socketQuarantine, error) {
	return nil, errSafeSocketCleanupUnavailable
}

func safeSocketQuarantineAvailable() bool {
	return false
}

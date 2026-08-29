//go:build !darwin && !linux

package supervisor

func newSocketQuarantine(string, *configuredSocketPathSet, socketCleanupHooks) (socketQuarantine, error) {
	return nil, errSafeSocketCleanupUnavailable
}

func socketDirectoryIdentityForPath(string) (socketDirectoryIdentity, error) {
	return socketDirectoryIdentity{}, errSafeSocketCleanupUnavailable
}

func safeSocketQuarantineAvailable() bool {
	return false
}

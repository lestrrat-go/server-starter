//go:build !linux

package supervisor

func newSocketQuarantine(string, *configuredSocketPathSet, socketCleanupHooks) (socketQuarantine, error) {
	return nil, errSafeSocketCleanupUnavailable
}

func socketDirectoryIdentityForPath(string) (socketDirectoryIdentity, error) {
	return socketDirectoryIdentity{}, errSafeSocketCleanupUnavailable
}

func socketIdentityForPath(string) (socketIdentity, error) {
	return socketIdentity{}, errSafeSocketCleanupUnavailable
}

func safeSocketQuarantineAvailable() bool {
	return false
}

func socketCleanupStateForPath(string, socketIdentity) (*socketCleanupState, error) {
	return nil, errSafeSocketCleanupUnavailable
}

func (s *socketCleanupState) remove(*configuredSocketPathSet) error {
	return nil
}

func (s *socketCleanupState) close() {}

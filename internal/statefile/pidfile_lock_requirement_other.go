//go:build !linux && !windows

package statefile

func requireLockOwner() bool {
	return false
}

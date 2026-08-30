//go:build windows

package statefile

func requireLockOwner() bool {
	return true
}

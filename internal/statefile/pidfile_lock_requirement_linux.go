//go:build linux

package statefile

// Linux can attribute a legacy flock through /proc/locks. If that inspection
// does not identify an owner, accepting a live recorded PID would allow a
// different process to receive control signals.
func requireLockOwner() bool {
	return true
}

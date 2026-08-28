//go:build !windows && !darwin && !linux

package supervisor

func platformTerminalWorkerStartError(error) bool {
	return false
}

//go:build !windows && !darwin && !linux

package supervisor

func platformTerminalWorkerStartError(string, string, error) bool {
	return false
}

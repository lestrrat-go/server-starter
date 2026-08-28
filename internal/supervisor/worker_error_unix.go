//go:build !windows && !darwin

package supervisor

func platformTerminalWorkerStartError(error) bool {
	return false
}

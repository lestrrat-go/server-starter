//go:build !windows

package control

import "syscall"

// signalProcess sends sig to the process identified by pid. It backs both
// --stop (SIGTERM) and --restart (SIGHUP).
func signalProcess(pid int, sig syscall.Signal) error {
	return syscall.Kill(pid, sig)
}

//go:build !windows

package cli

import "syscall"

// daemonSysProcAttr builds the SysProcAttr used to detach the daemonized
// child into its own session, so it survives the parent exiting and is not
// killed by a signal sent to the parent's process group.
func daemonSysProcAttr() (*syscall.SysProcAttr, error) {
	return &syscall.SysProcAttr{Setsid: true}, nil
}

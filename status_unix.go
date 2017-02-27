// +build !windows

package starter

import "syscall"

func init() {
	failureStatus = syscall.WaitStatus(0)
}

// +build !windows

package starter

import "syscall"

func init() {
	failureStatus = syscall.WaitStatus(255)
	successStatus = syscall.WaitStatus(0)
}

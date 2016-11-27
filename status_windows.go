// +build windows

package starter

import "syscall"

func init() {
	failureStatus = syscall.WaitStatus{ExitCode: 255}
	successStatus = syscall.WaitStatus{ExitCode: 0}
}

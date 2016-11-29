package starter

import "syscall"

func init() {
	failureStatus = syscall.WaitStatus{ExitCode: 255}
}

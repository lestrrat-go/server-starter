package starter

import "syscall"

func init() {
	failureStatus = syscall.WaitStatus{ExitCode: 255}
	successStatus = syscall.WaitStatus{ExitCode: 0}
}

func addPlatformDependentNiceSigNames(v map[syscall.Signal]string) map[syscall.Signal]string {
	return v
}

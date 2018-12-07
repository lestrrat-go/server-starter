package starter

import (
	"os"
	"syscall"
)

func init() {
	failureStatus = syscall.WaitStatus{ExitCode: 255}
	successStatus = syscall.WaitStatus{ExitCode: 0}
}

func addPlatformDependentNiceSigNames(v map[syscall.Signal]string) map[syscall.Signal]string {
	return v
}

func findWorker(pid int) *os.Process {
	p, err := os.FindProcess(pid)
	if err != nil {
		return p
	}
	return nil
}

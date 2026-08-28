package supervisor

import (
	"os"
	"syscall"
)

func init() {
	failureStatus = syscall.WaitStatus{ExitCode: 255}
	successStatus = syscall.WaitStatus{ExitCode: 0}
}

func findWorker(pid int) *os.Process {
	p, err := os.FindProcess(pid)
	if err != nil {
		return nil
	}
	return p
}

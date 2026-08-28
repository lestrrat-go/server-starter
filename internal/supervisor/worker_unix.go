//go:build !windows

package supervisor

import (
	"os"
	"syscall"
)

func init() {
	failureStatus = syscall.WaitStatus(255)
	successStatus = syscall.WaitStatus(0)
}

func findWorker(pid int) *os.Process {
	var wstatus syscall.WaitStatus
	waitpid, _ := syscall.Wait4(pid, &wstatus, syscall.WNOHANG, nil)
	if waitpid <= 0 {
		p, _ := os.FindProcess(pid)
		return p
	}
	return nil
}

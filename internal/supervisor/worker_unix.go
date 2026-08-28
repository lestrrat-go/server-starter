//go:build !windows

package supervisor

import (
	"os"
	"syscall"
)

func platformTerminalWorkerStartError(error) bool {
	return false
}

func init() {
	failureStatus = syscall.WaitStatus(255)
	successStatus = syscall.WaitStatus(0)
}

// findWorker probes whether the worker at pid is still alive. A non-nil
// *os.Process means it is; that meaning is unchanged.
//
// On Linux/Unix, the WNOHANG wait4 probe below both checks and reaps: if
// the worker has already died, this call consumes its exit status, and
// nothing else ever gets a chance to collect it (in particular, the
// caller's later cmd.Wait() finds nothing and leaves cmd.ProcessState nil).
// So when the reap happens here (waitpid > 0), the reaped wstatus is
// returned as the third value alongside a true ok flag, giving the caller
// the only place that status is ever observable.
func findWorker(pid int) (*os.Process, syscall.WaitStatus, bool) {
	var wstatus syscall.WaitStatus
	waitpid, _ := syscall.Wait4(pid, &wstatus, syscall.WNOHANG, nil)
	if waitpid > 0 {
		return nil, wstatus, true
	}
	p, _ := os.FindProcess(pid)
	return p, syscall.WaitStatus(0), false
}

package supervisor

import (
	"os"
	"syscall"
)

func init() {
	failureStatus = syscall.WaitStatus{ExitCode: 255}
	successStatus = syscall.WaitStatus{ExitCode: 0}
}

// findWorker probes whether the worker at pid is still alive. A non-nil
// *os.Process means it is; that meaning is unchanged.
//
// Unlike worker_unix.go's Wait4-based probe, Windows has no reap-on-probe
// race: this never consumes an exit status, so the third and fourth return
// values are always the zero WaitStatus and false.
func findWorker(pid int) (*os.Process, syscall.WaitStatus, bool) {
	p, err := os.FindProcess(pid)
	if err != nil {
		return nil, syscall.WaitStatus{}, false
	}
	return p, syscall.WaitStatus{}, false
}

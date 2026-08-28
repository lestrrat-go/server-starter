package control

import (
	"fmt"
	"syscall"
)

// signalProcess backs both --stop (SIGTERM) and --restart (SIGHUP). Neither
// has a Windows equivalent: there is no signal delivery to an arbitrary
// process by pid, and start_server's SIGHUP-triggered graceful restart has
// no counterpart in the Windows process model. Fail loudly instead of
// silently doing nothing or something surprising.
func signalProcess(pid int, sig syscall.Signal) error {
	return fmt.Errorf("signalling a process by pid is not supported on windows")
}

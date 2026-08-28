package main

import (
	"fmt"
	"syscall"
)

// daemonSysProcAttr has no Windows equivalent: Setsid (detaching into a new
// unix session) does not translate, and --daemonize's fork-and-detach model
// does not fit the Windows process model either. Fail loudly rather than
// launch a child that does not actually detach the way callers expect.
func daemonSysProcAttr() (*syscall.SysProcAttr, error) {
	return nil, fmt.Errorf("--daemonize is not supported on windows")
}

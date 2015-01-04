// +build !windows

package starter

import "syscall"

func init() {
	failureStatus = syscall.WaitStatus(255)
	successStatus = syscall.WaitStatus(0)
}

func addPlatformDependentNiceSigNames(v map[syscall.Signal]string) map[syscall.Signal]string {
	v[syscall.SIGCHLD] = "CHLD"
	v[syscall.SIGCONT] = "CONT"
	v[syscall.SIGIO] = "IO"
	v[syscall.SIGPROF] = "PROF"
	v[syscall.SIGSTOP] = "STOP"
	v[syscall.SIGSYS] = "SYS"
	v[syscall.SIGTSTP] = "TSTP"
	v[syscall.SIGTTIN] = "TTIN"
	v[syscall.SIGTTOU] = "TTOU"
	v[syscall.SIGURG] = "URG"
	v[syscall.SIGUSR1] = "USR1"
	v[syscall.SIGUSR2] = "USR2"
	v[syscall.SIGVTALRM] = "VTALRM"
	v[syscall.SIGWINCH] = "WINCH"
	v[syscall.SIGXCPU] = "XCPU"
	v[syscall.SIGXFSZ] = "GXFSZ"
	return v
}
//go:build darwin

package supervisor

import (
	"syscall"
)

var platformWorkerStartErrorPolicy = workerStartErrorPolicy{
	terminalErrors: []error{
		syscall.EBADEXEC,
		syscall.EBADARCH,
		syscall.ESHLIBVERS,
		syscall.EBADMACHO,
	},
}

//go:build darwin

package supervisor

import (
	"errors"
	"syscall"
)

func platformTerminalWorkerStartError(_, _ string, err error) bool {
	return errors.Is(err, syscall.EBADEXEC) ||
		errors.Is(err, syscall.EBADARCH) ||
		errors.Is(err, syscall.ESHLIBVERS) ||
		errors.Is(err, syscall.EBADMACHO)
}

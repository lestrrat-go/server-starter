//go:build linux

package supervisor

import (
	"errors"
	"os"
	"syscall"
)

func platformTerminalWorkerStartError(err error) bool {
	var pathErr *os.PathError
	return errors.As(err, &pathErr) &&
		pathErr.Op == "fork/exec" &&
		errors.Is(err, syscall.EIO)
}

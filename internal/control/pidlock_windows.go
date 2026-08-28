package control

import (
	"fmt"
	"os"
)

// tryLockPIDFile is used by Stop to poll for the supervisor having
// exited. --stop itself is unsupported on Windows (see signal_windows.go),
// so this is unreachable in practice; it exists to keep the platform seam
// symmetric and to fail loudly rather than silently if that ever changes.
func tryLockPIDFile(f *os.File) error {
	return fmt.Errorf("waiting for a stopped process is not supported on windows")
}

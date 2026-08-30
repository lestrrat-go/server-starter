package supervisor

import (
	"fmt"
	"os"
	"runtime"
	"strings"
)

// validateUnixSocketPathAvailable enforces the fail-closed filesystem
// lifecycle contract. Existing entries are never removed or replaced because
// a pathname check cannot be bound to the same entry at a later operation.
func validateUnixSocketPathAvailable(path string) error {
	if path == "" || (runtime.GOOS == "linux" &&
		(strings.HasPrefix(path, "@") || strings.HasPrefix(path, "\x00"))) {
		return nil
	}

	_, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect unix socket path %q: %w", path, err)
	}
	return fmt.Errorf("unix socket path %q already exists; remove it before starting", path)
}

package supervisor

import "errors"

var errSafeSocketCleanupUnavailable = errors.New("identity-safe unix socket cleanup is unavailable on this platform")

type socketQuarantine interface {
	moveIn() error
	entryIsSocket() (bool, error)
	restore() error
	removeEntry() error
	cleanup() error
	close()
	location() string
}

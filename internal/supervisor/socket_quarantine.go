package supervisor

import "errors"

const (
	quarantineDirPrefix   = ".server-starter-socket-"
	quarantineDirName     = quarantineDirPrefix + "quarantine"
	quarantineEntryPrefix = "socket-"
)

var errSafeSocketCleanupUnavailable = errors.New("identity-safe unix socket cleanup is unavailable on this platform")

var errSocketSourceUnavailable = errors.New("unix socket path was not present when cleanup started")

var errSocketSourceChanged = errors.New("unix socket path changed before quarantine")

var errIdentitySafeSocketRemovalUnavailable = errors.New(
	"identity-safe removal of a quarantined unix socket is unavailable",
)

type socketQuarantine interface {
	moveIn() error
	entryIsSocket() (bool, error)
	restore() error
	removeEntry() error
	cleanup() error
	close()
	location() string
}

package supervisor

import "errors"

const (
	quarantineDirPrefix = ".server-starter-socket-"
	quarantineDirName   = quarantineDirPrefix + "quarantine"
	quarantineEntryName = "socket"
)

var errSafeSocketCleanupUnavailable = errors.New("identity-safe unix socket cleanup is unavailable on this platform")

var errSocketSourceUnavailable = errors.New("unix socket path was not present when cleanup started")

var errSocketSourceChanged = errors.New("unix socket path changed before quarantine")

type socketQuarantine interface {
	moveIn() error
	entryIsSocket() (bool, error)
	restore() error
	removeEntry() error
	cleanup() error
	close()
	location() string
}

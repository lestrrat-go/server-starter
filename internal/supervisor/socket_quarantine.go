package supervisor

import (
	"errors"
	"path/filepath"
)

const (
	quarantineDirPrefix   = ".server-starter-socket-"
	quarantineDirName     = quarantineDirPrefix + "quarantine"
	quarantineEntryPrefix = "socket-"
)

var errSafeSocketCleanupUnavailable = errors.New("identity-safe unix socket cleanup is unavailable on this platform")

var errSocketSourceUnavailable = errors.New("unix socket path was not present when cleanup started")

var errSocketSourceChanged = errors.New("unix socket path changed before quarantine")

type socketQuarantine interface {
	moveIn() error
	entryIsSocket() (bool, error)
	restore() error
	retainEntry() error
	cleanup() error
	close()
	location() string
}

func configuredSocketBasenames(paths []string) map[string]struct{} {
	basenames := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		basenames[filepath.Base(path)] = struct{}{}
	}
	return basenames
}

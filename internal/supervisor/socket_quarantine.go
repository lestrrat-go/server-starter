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

func configuredSocketPaths(paths []string) map[string]struct{} {
	configured := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		configured[normalizeSocketPath(path)] = struct{}{}
	}
	return configured
}

func normalizeSocketPath(path string) string {
	absPath, err := filepath.Abs(path)
	if err == nil {
		return absPath
	}
	return filepath.Clean(path)
}

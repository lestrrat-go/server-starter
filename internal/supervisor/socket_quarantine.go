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
	entryMatchesIdentity(socketIdentity) (bool, error)
	restore() error
	removeEntry() error
	retainEntry() error
	cleanup() error
	close()
	location() string
}

type socketIdentity struct {
	device uint64
	inode  uint64
}

type socketDirectoryIdentity struct {
	device uint64
	inode  uint64
}

type configuredSocketPathIdentity struct {
	parent socketDirectoryIdentity
	name   string
}

type configuredSocketPathSet struct {
	identities map[configuredSocketPathIdentity]struct{}
	lexical    map[string]struct{}
}

func configuredSocketPaths(paths []string) *configuredSocketPathSet {
	configured := &configuredSocketPathSet{
		identities: make(map[configuredSocketPathIdentity]struct{}, len(paths)),
		lexical:    make(map[string]struct{}, len(paths)),
	}
	for _, path := range paths {
		configured.lexical[normalizeSocketPath(path)] = struct{}{}
		parentPath, name := filepath.Split(path)
		if parentPath == "" {
			parentPath = "." + string(filepath.Separator)
		}
		parent, err := socketDirectoryIdentityForPath(parentPath)
		if err == nil {
			configured.identities[configuredSocketPathIdentity{parent: parent, name: name}] = struct{}{}
		}
	}
	return configured
}

func (s *configuredSocketPathSet) contains(parent socketDirectoryIdentity, name, path string) bool {
	if s == nil {
		return false
	}
	if _, configured := s.identities[configuredSocketPathIdentity{parent: parent, name: name}]; configured {
		return true
	}
	_, configured := s.lexical[normalizeSocketPath(path)]
	return configured
}

func normalizeSocketPath(path string) string {
	absPath, err := filepath.Abs(path)
	if err == nil {
		return absPath
	}
	return filepath.Clean(path)
}

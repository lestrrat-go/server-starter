package supervisor

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var errNoEnv = errors.New("no ENVDIR specified, or ENVDIR does not exist")

// loadEnvdir reads the envdir at dn and returns the resulting variable map
// for the caller to overlay onto a spawned worker's environment. Unlike the
// old setEnv, it is a pure function: it never touches the supervisor's own
// process environment, so the caller decides what, if anything, to do with
// the result.
func loadEnvdir(dn string) map[string]string {
	m, err := reloadEnv(dn)
	if err != nil && !errors.Is(err, errNoEnv) {
		fmt.Fprintf(os.Stderr, "failed to load from envdir: %s\n", err)
	}
	return m
}

// reloadEnv reads dn (one file per variable; a file's value is the first
// line of its contents) into a map. It returns errNoEnv when dn is empty,
// does not exist, or contains no usable entries.
func reloadEnv(dn string) (map[string]string, error) {
	if dn == "" {
		return nil, errNoEnv
	}

	entries, err := os.ReadDir(dn)
	if err != nil {
		return nil, errNoEnv
	}

	m := make(map[string]string)
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".") || entry.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dn, name))
		if err != nil || len(data) == 0 {
			continue
		}
		value := string(data)
		if i := strings.IndexByte(value, '\n'); i >= 0 {
			value = value[:i]
		}
		m[name] = value
	}

	if len(m) == 0 {
		return nil, errNoEnv
	}

	return m, nil
}

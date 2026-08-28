package starter

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

var errNoEnv = errors.New("no ENVDIR specified, or ENVDIR does not exist")
var envMu sync.Mutex
var managedEnv = make(map[string]struct{})

func setEnv() {
	envMu.Lock()
	defer envMu.Unlock()

	m, err := reloadEnv()
	if err != nil && err != errNoEnv {
		fmt.Fprintf(os.Stderr, "failed to load from envdir: %s\n", err)
	}
	for name := range managedEnv {
		if _, ok := m[name]; !ok {
			_ = os.Unsetenv(name)
		}
	}
	for name, value := range m {
		_ = os.Setenv(name, value)
		managedEnv[name] = struct{}{}
	}
	for name := range managedEnv {
		if _, ok := m[name]; !ok {
			delete(managedEnv, name)
		}
	}
}

func reloadEnv() (map[string]string, error) {
	dn := os.Getenv("ENVDIR")
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

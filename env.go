package starter

import (
	"bufio"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

var errNoEnv = errors.New("no ENVDIR specified, or ENVDIR does not exist")

func reloadEnv() (map[string]string, error) {
	dn := os.Getenv("ENVDIR")
	if dn == "" {
		return nil, errNoEnv
	}

	fi, err := os.Stat(dn)
	if err != nil {
		return nil, errNoEnv
	}

	if !fi.IsDir() {
		return nil, errNoEnv
	}

	var m map[string]string

	filepath.Walk(dn, func(path string, fi os.FileInfo, err error) error {
		// Ignore errors
		if err != nil {
			return nil
		}

		// Don't go into directories
		if fi.IsDir() && dn != path {
			return filepath.SkipDir
		}

		f, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer f.Close()

		envName := filepath.Base(path)
		scanner := bufio.NewScanner(f)
		if scanner.Scan() {
			if m == nil {
				m = make(map[string]string)
			}
			l := scanner.Text()
			m[envName] = strings.TrimSpace(l)
		}

		return nil
	})

	if m == nil {
		return nil, errNoEnv
	}

	return m, nil
}

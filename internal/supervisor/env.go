package supervisor

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

var errNoEnv = errors.New("no ENVDIR specified, or ENVDIR does not exist")

const maxEnvValueBytes = 128 * 1024

// loadEnvdir reads the envdir at dn and returns the resulting variable map
// for the caller to overlay onto a spawned worker's environment. Unlike the
// old setEnv, it is a pure function: it never touches the supervisor's own
// process environment, so the caller decides what, if anything, to do with
// the result. Any failure to load is reported to w, since loadEnvdir has no
// stream of its own.
func loadEnvdir(dn string, w io.Writer) map[string]string {
	m, err := reloadEnv(dn)
	if err != nil && !errors.Is(err, errNoEnv) {
		fmt.Fprintf(w, "failed to load from envdir: %s\n", err)
	}
	return m
}

// reloadEnv reads dn (one regular file per variable; a file's value is its
// first line, up to maxEnvValueBytes) into a map. It returns errNoEnv when dn
// is empty, does not exist, or contains no usable entries.
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
		if strings.HasPrefix(name, ".") || entry.Type()&os.ModeType != 0 {
			continue
		}

		file, err := openEnvFile(filepath.Join(dn, name))
		if err != nil {
			continue
		}
		value, ok := readEnvValue(file)
		if err := file.Close(); err != nil || !ok {
			continue
		}
		m[name] = value
	}

	if len(m) == 0 {
		return nil, errNoEnv
	}

	return m, nil
}

func readEnvValue(file *os.File) (string, bool) {
	data, err := io.ReadAll(io.LimitReader(file, maxEnvValueBytes+1))
	if err != nil || len(data) == 0 {
		return "", false
	}

	if line, _, found := bytes.Cut(data, []byte("\n")); found {
		return string(line), true
	}
	if len(data) > maxEnvValueBytes {
		return "", false
	}
	return string(data), true
}

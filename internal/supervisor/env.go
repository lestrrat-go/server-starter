package supervisor

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const maxEnvValueBytes = 128 * 1024

// reloadEnv reads dn (one regular file per variable; a file's value is its
// first line, up to maxEnvValueBytes) into a map. An empty dn disables envdir
// loading.
func reloadEnv(dn string) (map[string]string, error) {
	if dn == "" {
		return map[string]string{}, nil
	}

	entries, err := os.ReadDir(dn)
	if err != nil {
		return nil, fmt.Errorf("read envdir %q: %w", dn, err)
	}

	m := make(map[string]string)
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".") || entry.Type()&os.ModeType != 0 {
			continue
		}

		path := filepath.Join(dn, name)
		file, err := openEnvFile(path)
		if err != nil {
			return nil, fmt.Errorf("read envdir entry %q: %w", path, err)
		}
		value, ok, readErr := readEnvValue(file)
		closeErr := file.Close()
		if readErr != nil {
			return nil, fmt.Errorf("read envdir entry %q: %w", path, readErr)
		}
		if closeErr != nil {
			return nil, fmt.Errorf("close envdir entry %q: %w", path, closeErr)
		}
		if !ok {
			continue
		}
		m[name] = value
	}

	return m, nil
}

func readEnvValue(file *os.File) (string, bool, error) {
	data, err := io.ReadAll(io.LimitReader(file, maxEnvValueBytes+1))
	if err != nil {
		return "", false, err
	}
	if len(data) == 0 {
		return "", false, nil
	}

	if line, _, found := bytes.Cut(data, []byte("\n")); found {
		return string(line), true, nil
	}
	if len(data) > maxEnvValueBytes {
		return "", false, nil
	}
	return string(data), true, nil
}

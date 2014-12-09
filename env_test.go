package starter

import (
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
)

func TestEnvdir(t *testing.T) {
	dir, err := ioutil.TempDir("", "starter_test")
	if err != nil {
		t.Errorf("Failed to create tempdir: %s", err)
		return
	}
	defer os.RemoveAll(dir)

	files := []string{"FOO", "BAR", "BAZ"}
	for _, fn := range files {
		longFn := filepath.Join(dir, fn)

		f, err := os.OpenFile(longFn, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0666)
		if err != nil {
			t.Errorf("Failed to create file '%s': %s", fn, err)
			return
		}
		closed := false
		defer func() {
			if !closed {
				f.Close()
			}
		}()

		io.WriteString(f, fn)
		f.Close()
		closed = true

		// save old values and restore later, if any
		if old := os.Getenv(fn); old != "" {
			os.Setenv(fn, "")
			defer os.Setenv(fn, old)
		}
	}

	if old := os.Getenv("ENVDIR"); old != "" {
		defer os.Setenv("ENVDIR", old)
	}

	os.Setenv("ENVDIR", dir)
	m, err := reloadEnv()
	if err != nil {
		t.Errorf("reloadEnv failed: %s", err)
		return
	}

	for _, fn := range files {
		v, ok := m[fn]
		if !ok {
			t.Errorf("Expected environment variable '%s' to exist")
			return
		}
		if v != fn {
			t.Errorf("Expected environment variable '%s' to be '%s'", fn, fn)
			return
		}
	}
}
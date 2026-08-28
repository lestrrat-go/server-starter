package supervisor

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnvdir(t *testing.T) {
	dir := t.TempDir()

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
	}

	m, err := reloadEnv(dir)
	if err != nil {
		t.Errorf("reloadEnv failed: %s", err)
		return
	}

	for _, fn := range files {
		v, ok := m[fn]
		if !ok {
			t.Errorf("Expected environment variable '%s' to exist", fn)
			return
		}
		if v != fn {
			t.Errorf("Expected environment variable '%s' to be '%s'", fn, fn)
			return
		}
	}
}

func TestReloadEnvdirCompatibility(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "VALUE"), []byte("  keep whitespace  \nignored"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".hidden"), []byte("hidden"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "EMPTY"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "LONG"), []byte(strings.Repeat("x", 128*1024)), 0600); err != nil {
		t.Fatal(err)
	}

	got, err := reloadEnv(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got["VALUE"] != "  keep whitespace  " {
		t.Fatalf("VALUE = %q", got["VALUE"])
	}
	if got["LONG"] != strings.Repeat("x", 128*1024) {
		t.Fatalf("LONG was not read in full")
	}
	if _, ok := got[".hidden"]; ok {
		t.Fatal("hidden envdir entries must be ignored")
	}
	if _, ok := got["EMPTY"]; ok {
		t.Fatal("empty envdir entries must be ignored")
	}
}

// TestLoadEnvdirDropsDeletedValues proves loadEnvdir's map reflects the
// envdir's current contents on every call, with no leftover bookkeeping
// from a previous call: a key removed from the envdir is simply absent
// from the next map, rather than needing to be explicitly unset anywhere.
// This replaces the old managedEnv tracking in setEnv, which existed only
// because that function mutated the supervisor's own process environment;
// loadEnvdir never does, so there is nothing to unset (see env.go).
func TestLoadEnvdirDropsDeletedValues(t *testing.T) {
	dir := t.TempDir()
	name := "SERVER_STARTER_TEST_ENV"
	if err := os.WriteFile(filepath.Join(dir, name), []byte("first\n"), 0600); err != nil {
		t.Fatal(err)
	}

	m := loadEnvdir(dir, io.Discard)
	if got := m[name]; got != "first" {
		t.Fatalf("initial envdir value = %q", got)
	}

	if err := os.Remove(filepath.Join(dir, name)); err != nil {
		t.Fatal(err)
	}

	m = loadEnvdir(dir, io.Discard)
	if _, ok := m[name]; ok {
		t.Fatal("deleted envdir entry remained in the reloaded map")
	}
}

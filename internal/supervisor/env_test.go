package supervisor

import (
	"context"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
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
	if err := os.WriteFile(filepath.Join(dir, "LONG"), []byte(strings.Repeat("x", maxEnvValueBytes)), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(dir, "FIRST_LINE"),
		[]byte("first\n"+strings.Repeat("x", maxEnvValueBytes+1)),
		0600,
	); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(dir, "TOO_LONG"),
		[]byte(strings.Repeat("x", maxEnvValueBytes+1)),
		0600,
	); err != nil {
		t.Fatal(err)
	}
	got, err := reloadEnv(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got["VALUE"] != "  keep whitespace  " {
		t.Fatalf("VALUE = %q", got["VALUE"])
	}
	if got["LONG"] != strings.Repeat("x", maxEnvValueBytes) {
		t.Fatalf("LONG was not read in full")
	}
	if got["FIRST_LINE"] != "first" {
		t.Fatalf("FIRST_LINE = %q", got["FIRST_LINE"])
	}
	if _, ok := got[".hidden"]; ok {
		t.Fatal("hidden envdir entries must be ignored")
	}
	if _, ok := got["EMPTY"]; ok {
		t.Fatal("empty envdir entries must be ignored")
	}
	if _, ok := got["TOO_LONG"]; ok {
		t.Fatal("oversized envdir entries must be ignored")
	}
}

func TestReloadEnvdirSkipsSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(t.TempDir(), "target")
	require.NoError(t, os.WriteFile(target, []byte("linked"), 0600))
	if err := os.Symlink(target, filepath.Join(dir, "LINK")); err != nil {
		t.Skipf("symlinks are unavailable: %s", err)
	}

	got, err := reloadEnv(dir)
	require.NoError(t, err)
	require.Empty(t, got)
}

func TestReloadEnvdirUnset(t *testing.T) {
	got, err := reloadEnv("")
	require.NoError(t, err)
	require.Empty(t, got)
}

func TestReloadEnvdirReportsDirectoryErrors(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "missing")
	notDirectory := filepath.Join(dir, "not-directory")
	require.NoError(t, os.WriteFile(notDirectory, []byte("value"), 0600))

	for name, path := range map[string]string{
		"missing":       missing,
		"not directory": notDirectory,
	} {
		t.Run(name, func(t *testing.T) {
			got, err := reloadEnv(path)
			require.Nil(t, got)
			require.Error(t, err)
			require.Contains(t, err.Error(), path)

			var pathErr *os.PathError
			require.ErrorAs(t, err, &pathErr)
			require.Equal(t, path, pathErr.Path)
		})
	}
}

// TestReloadEnvdirDropsDeletedValues proves reloadEnv's map reflects the
// envdir's current contents on every call, with no leftover bookkeeping
// from a previous call: a key removed from the envdir is simply absent
// from the next map, rather than needing to be explicitly unset anywhere.
// This replaces the old managedEnv tracking in setEnv, which existed only
// because that function mutated the supervisor's own process environment;
// reloadEnv never does, so there is nothing to unset (see env.go).
func TestReloadEnvdirDropsDeletedValues(t *testing.T) {
	dir := t.TempDir()
	name := "SERVER_STARTER_TEST_ENV"
	if err := os.WriteFile(filepath.Join(dir, name), []byte("first\n"), 0600); err != nil {
		t.Fatal(err)
	}

	m, err := reloadEnv(dir)
	require.NoError(t, err)
	if got := m[name]; got != "first" {
		t.Fatalf("initial envdir value = %q", got)
	}

	if err := os.Remove(filepath.Join(dir, name)); err != nil {
		t.Fatal(err)
	}

	m, err = reloadEnv(dir)
	require.NoError(t, err)
	if _, ok := m[name]; ok {
		t.Fatal("deleted envdir entry remained in the reloaded map")
	}
}

func TestRunRejectsMissingEnvdir(t *testing.T) {
	command, err := os.Executable()
	require.NoError(t, err)

	path := filepath.Join(t.TempDir(), "missing")
	sd, err := NewStarter(&config{command: command, envdir: path})
	require.NoError(t, err)

	ctrl, err := sd.Run(context.Background())
	require.Nil(t, ctrl)
	require.ErrorIs(t, err, fs.ErrNotExist)
	require.Contains(t, err.Error(), path)
}

package starter

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAcquirePIDFileWritesNewlineAndRemovesOwnedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "server.pid")
	pid, err := acquirePIDFile(path)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(string(data), "\n") {
		t.Fatalf("pid file %q has no trailing newline", data)
	}
	if err := pid.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("pid file still exists, stat error = %v", err)
	}
}

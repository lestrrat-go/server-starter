package supervisor

import (
	"net"
	"os"
	"path/filepath"
	"testing"
)

func TestTeardownRemovesUnixSocket(t *testing.T) {
	path := filepath.Join(t.TempDir(), "server.sock")
	l, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	s := &Starter{listeners: []listener{{listener: l, spec: path}}}
	s.teardown()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("unix socket path remains, stat error = %v", err)
	}
}

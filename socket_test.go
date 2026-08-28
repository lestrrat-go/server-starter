package starter

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
	if err := s.Teardown(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("unix socket path remains, stat error = %v", err)
	}
}

func TestParsePortTarget(t *testing.T) {
	for _, test := range []struct {
		name, spec, network, host string
		port, fd                  int
	}{
		{name: "tcp4", spec: "8080", network: "tcp4", port: 8080, fd: -1},
		{name: "udp4", spec: "u8080", network: "udp4", port: 8080, fd: -1},
		{name: "tcp6", spec: "[::1]:8080", network: "tcp6", host: "::1", port: 8080, fd: -1},
		{name: "udp6", spec: "u[::1]:8080=7", network: "udp6", host: "::1", port: 8080, fd: 7},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := parsePortTarget(test.spec)
			if err != nil {
				t.Fatal(err)
			}
			if got.network != test.network || got.host != test.host || got.port != test.port || got.fd != test.fd {
				t.Fatalf("target = %#v", got)
			}
		})
	}
}

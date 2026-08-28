package starter

import (
	"net"
	"os"
	"path/filepath"
	"testing"
)

func TestParsePortTarget(t *testing.T) {
	tests := []struct {
		name    string
		spec    string
		network string
		host    string
		port    int
	}{
		{name: "tcp4", spec: "8080", network: "tcp4", port: 8080},
		{name: "udp4", spec: "u8080", network: "udp4", port: 8080},
		{name: "udp4 host suffix", spec: "127.0.0.1:u8080", network: "udp4", host: "127.0.0.1", port: 8080},
		{name: "tcp6", spec: "[::1]:8080", network: "tcp6", host: "::1", port: 8080},
		{name: "udp6", spec: "u[::1]:8080", network: "udp6", host: "::1", port: 8080},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := parsePortTarget(test.spec)
			if err != nil {
				t.Fatal(err)
			}
			if got.network != test.network || got.host != test.host || got.port != test.port {
				t.Fatalf("target = %#v", got)
			}
		})
	}
}

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

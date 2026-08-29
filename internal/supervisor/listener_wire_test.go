package supervisor

// This test lives in the internal `supervisor` package (not
// `supervisor_test`) because it exercises the unexported listener type and
// parsePortTarget.

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	starter "github.com/lestrrat-go/server-starter/v2"
	"github.com/stretchr/testify/require"
)

// TestPortSpecWireFormat guards the shared SERVER_STARTER_PORT contract
// between the supervisor's parser and FormatPorts, including canonicalising
// legacy UDP spellings to the explicit udp:// marker.
func TestPortSpecWireFormat(t *testing.T) {
	const fd = 3

	tcpUDPCases := []struct {
		name string
		raw  string
		want string
	}{
		{name: "tcp bare port", raw: "8080", want: "8080=3"},
		{name: "tcp host:port", raw: "127.0.0.1:9090", want: "127.0.0.1:9090=3"},
		{name: "tcp hostname beginning with u", raw: "ubuntu.internal:9090", want: "ubuntu.internal:9090=3"},
		{name: "tcp ipv6", raw: "[::1]:9090", want: "[::1]:9090=3"},
		{name: "udp bare port", raw: "udp://8080", want: "udp://8080=3"},
		{name: "udp host:port", raw: "udp://127.0.0.1:9090", want: "udp://127.0.0.1:9090=3"},
		{name: "legacy udp bare port", raw: "u8080", want: "udp://8080=3"},
	}
	for _, tc := range tcpUDPCases {
		t.Run(tc.name, func(t *testing.T) {
			target, err := parsePortTarget(tc.raw)
			require.NoError(t, err)

			l := listener{network: target.network, host: target.host, port: target.port}
			got, err := starter.FormatPorts(l.starterListener(fd))
			require.NoError(t, err)

			require.Equal(t, tc.want, got)
			require.Equal(t, strings.TrimSuffix(tc.want, "=3"), target.spec)
		})
	}

	const unixFD = 5
	unixCases := []struct {
		name string
		path string
	}{
		{name: "unix absolute", path: "/tmp/app.sock"},
		{name: "unix relative", path: "rel.sock"},
	}
	for _, tc := range unixCases {
		t.Run(tc.name, func(t *testing.T) {
			// old code: fmt.Sprintf("%s=%d", l.spec, descriptors[i]), where
			// a unix listener's spec was the raw path, unmodified.
			want := fmt.Sprintf("%s=%d", tc.path, unixFD)

			l := listener{network: unixNetwork, path: tc.path}
			got, err := starter.FormatPorts(l.starterListener(unixFD))
			require.NoError(t, err)

			require.Equal(t, want, got)
		})
	}
}

func TestRunRejectsUnixPathsReservedByWireFormatBeforeBinding(t *testing.T) {
	for _, tc := range []struct {
		name     string
		fileName string
	}{
		{name: "list delimiter", fileName: "app;next.sock"},
		{name: "pair delimiter", fileName: "app=next.sock"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			validPath := filepath.Join(dir, "valid.sock")
			invalidPath := filepath.Join(dir, tc.fileName)
			_, wantErr := starter.FormatPorts(starter.NewUnixListener(invalidPath, 4))
			require.Error(t, wantErr)
			s := &Starter{
				paths:  []string{validPath, invalidPath},
				stderr: io.Discard,
			}

			ctrl, err := s.Run(context.Background())
			require.Nil(t, ctrl)
			require.EqualError(t, err, wantErr.Error())
			_, statErr := os.Lstat(validPath)
			require.ErrorIs(t, statErr, os.ErrNotExist)
			_, statErr = os.Lstat(invalidPath)
			require.ErrorIs(t, statErr, os.ErrNotExist)
		})
	}
}

func TestRunRejectsPortAddressesReservedByWireFormatBeforeBinding(t *testing.T) {
	for _, tc := range []struct {
		name     string
		raw      string
		listener starter.Listener
	}{
		{
			name:     "TCP address",
			raw:      "host;next:8080",
			listener: starter.NewTCPListener("host;next", 8080, 3),
		},
		{
			name:     "UDP address",
			raw:      "udp://host;next:8080",
			listener: starter.NewUDPListener("host;next", 8080, 3),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			validPath := filepath.Join(dir, "valid.sock")
			_, wantErr := starter.FormatPorts(tc.listener)
			require.Error(t, wantErr)
			s := &Starter{
				ports:  []string{tc.raw},
				paths:  []string{validPath},
				stderr: io.Discard,
			}

			ctrl, err := s.Run(context.Background())
			require.Nil(t, ctrl)
			require.EqualError(t, err, wantErr.Error())
			_, statErr := os.Lstat(validPath)
			require.ErrorIs(t, statErr, os.ErrNotExist)
		})
	}
}

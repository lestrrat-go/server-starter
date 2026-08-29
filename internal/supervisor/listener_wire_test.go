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
	"testing"

	starter "github.com/lestrrat-go/server-starter/v2"
	"github.com/stretchr/testify/require"
)

// TestPortSpecWireFormatUnchanged guards startWorker's switch from an
// inline fmt.Sprintf("%s=%d", spec, fd) to a starter.List built from typed
// Listener values (NewTCPListener/NewUDPListener/NewUnixListener) and
// formatted through starter.FormatPorts. SERVER_STARTER_PORT is the wire
// protocol between this supervisor and every worker, including ones built
// with a different version of this module; the two formatting paths must
// stay byte-identical for every shape the supervisor can emit, or a future
// refactor could silently break that protocol.
func TestPortSpecWireFormatUnchanged(t *testing.T) {
	const fd = 3

	tcpUDPCases := []struct {
		name string
		raw  string
	}{
		{name: "tcp bare port", raw: "8080"},
		{name: "tcp host:port", raw: "127.0.0.1:9090"},
		{name: "tcp ipv6", raw: "[::1]:9090"},
		{name: "udp bare port", raw: "u8080"},
		{name: "udp host:port", raw: "u127.0.0.1:9090"},
	}
	for _, tc := range tcpUDPCases {
		t.Run(tc.name, func(t *testing.T) {
			target, err := parsePortTarget(tc.raw)
			require.NoError(t, err)

			// old code: fmt.Sprintf("%s=%d", l.spec, descriptors[i])
			want := fmt.Sprintf("%s=%d", target.spec, fd)

			l := listener{network: target.network, host: target.host, port: target.port}
			got, err := starter.FormatPorts(l.starterListener(fd))
			require.NoError(t, err)

			require.Equal(t, want, got)
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

			l := listener{network: "unix", path: tc.path}
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
			raw:      "uhost;next:8080",
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

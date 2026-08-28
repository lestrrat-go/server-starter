package supervisor

// This test lives in the internal `supervisor` package (not
// `supervisor_test`) because it exercises the unexported listener type and
// parsePortTarget.

import (
	"fmt"
	"testing"

	starter "github.com/lestrrat-go/server-starter/v2"
	"github.com/stretchr/testify/require"
)

// TestPortSpecWireFormatUnchanged guards startWorker's switch from an
// inline fmt.Sprintf("%s=%d", spec, fd) to a starter.List built from typed
// Listener values (NewTCPListener/NewUDPListener/NewUnixListener) and
// formatted through List.String(). SERVER_STARTER_PORT is the wire
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
			got := starter.List{l.starterListener(fd)}.String()

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
			got := starter.List{l.starterListener(unixFD)}.String()

			require.Equal(t, want, got)
		})
	}
}

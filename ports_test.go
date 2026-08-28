package starter_test

import (
	"errors"
	"fmt"
	"net"
	"testing"

	"github.com/stretchr/testify/require"

	starter "github.com/lestrrat-go/server-starter/v2"
)

// fakeListener is a starter.Listener implementation that is neither
// TCPListener, UDPListener, nor UnixListener, used to exercise
// FormatPorts's handling of an unknown Listener implementation.
type fakeListener struct{}

const addrField = "Addr"

func (fakeListener) Fd() uintptr { return 0 }
func (fakeListener) Listen() (net.Listener, error) {
	return nil, errors.New("fakeListener: Listen not implemented")
}
func (fakeListener) String() string { return "fake" }

func TestPorts(t *testing.T) {
	expect := starter.List{
		starter.NewTCPListener("127.0.0.1", 9090, 4),
		starter.NewTCPListener("", 8080, 5),
		starter.NewUnixListener("/foo/bar/baz.sock", 6),
	}

	spec, err := starter.FormatPorts(expect...)
	require.NoError(t, err)
	t.Setenv(starter.PortEnvName, spec)
	ports, err := starter.Ports()
	require.NoError(t, err)
	require.Len(t, ports, len(expect))

	for i, port := range ports {
		require.Equal(t, expect[i].Fd(), port.Fd())
		_, gotTCP := port.(starter.TCPListener)
		_, expectTCP := expect[i].(starter.TCPListener)
		require.Equal(t, expectTCP, gotTCP)
	}
}

func TestPortsNoEnv(t *testing.T) {
	t.Setenv(starter.PortEnvName, "")

	ports, err := starter.Ports()
	require.ErrorIs(t, err, starter.ErrNoListeningTarget)
	require.Nil(t, ports)
}

func TestParsePorts(t *testing.T) {
	t.Run("bare port", func(t *testing.T) {
		got, err := starter.ParsePorts("8080=3")
		require.NoError(t, err)
		require.Len(t, got, 1)
		require.IsType(t, starter.TCPListener{}, got[0])
		require.Equal(t, starter.NewTCPListener("0.0.0.0", 8080, 3), got[0])
	})

	t.Run("host and port", func(t *testing.T) {
		got, err := starter.ParsePorts("127.0.0.1:9090=4")
		require.NoError(t, err)
		require.Len(t, got, 1)
		require.IsType(t, starter.TCPListener{}, got[0])
		require.Equal(t, starter.NewTCPListener("127.0.0.1", 9090, 4), got[0])
	})

	t.Run("IPv6 host and port", func(t *testing.T) {
		got, err := starter.ParsePorts("[::1]:9090=4")
		require.NoError(t, err)
		require.Equal(t, starter.NewTCPListener("::1", 9090, 4), got[0])
		require.Equal(t, "[::1]:9090=4", got[0].String())
	})

	t.Run("UDP host and port", func(t *testing.T) {
		got, err := starter.ParsePorts("udp://127.0.0.1:9090=4")
		require.NoError(t, err)
		require.Equal(t, starter.NewUDPListener("127.0.0.1", 9090, 4), got[0])
	})

	t.Run("TCP hostname beginning with u", func(t *testing.T) {
		got, err := starter.ParsePorts("ubuntu.internal:8080=4")
		require.NoError(t, err)
		require.Equal(t, starter.NewTCPListener("ubuntu.internal", 8080, 4), got[0])
	})

	t.Run("legacy UDP bare port", func(t *testing.T) {
		got, err := starter.ParsePorts("u8080=4")
		require.NoError(t, err)
		require.Equal(t, starter.NewUDPListener("", 8080, 4), got[0])
	})

	t.Run("UDP port suffix", func(t *testing.T) {
		got, err := starter.ParsePorts("127.0.0.1:u9090=4")
		require.NoError(t, err)
		require.Equal(t, starter.NewUDPListener("127.0.0.1", 9090, 4), got[0])
	})

	t.Run("UDP IPv6 host and port", func(t *testing.T) {
		got, err := starter.ParsePorts("udp://[::1]:9090=4")
		require.NoError(t, err)
		require.Equal(t, starter.NewUDPListener("::1", 9090, 4), got[0])
		require.Equal(t, "udp://[::1]:9090=4", got[0].String())
	})

	t.Run("legacy UDP IPv6 host and port", func(t *testing.T) {
		got, err := starter.ParsePorts("u[::1]:9090=4")
		require.NoError(t, err)
		require.Equal(t, starter.NewUDPListener("::1", 9090, 4), got[0])
	})

	t.Run("UDP host and port with marker in both positions", func(t *testing.T) {
		got, err := starter.ParsePorts("u10.0.0.5:u9090=4")
		require.NoError(t, err)
		require.Equal(t, starter.NewUDPListener("10.0.0.5", 9090, 4), got[0])
	})

	t.Run("unix socket path", func(t *testing.T) {
		got, err := starter.ParsePorts("/foo/bar.sock=5")
		require.NoError(t, err)
		require.Len(t, got, 1)
		require.IsType(t, starter.UnixListener{}, got[0])
		require.Equal(t, starter.NewUnixListener("/foo/bar.sock", 5), got[0])
	})

	t.Run("missing equals sign", func(t *testing.T) {
		got, err := starter.ParsePorts("8080")
		require.Error(t, err)
		require.Nil(t, got)
	})

	t.Run("multiple equals signs", func(t *testing.T) {
		got, err := starter.ParsePorts("8080=3=extra")
		require.Error(t, err)
		require.Nil(t, got)
	})

	t.Run("empty string", func(t *testing.T) {
		got, err := starter.ParsePorts("")
		require.ErrorIs(t, err, starter.ErrNoListeningTarget)
		require.Nil(t, got)
	})

	t.Run("relative unix path starting with u is not stripped", func(t *testing.T) {
		got, err := starter.ParsePorts("unix.sock=5")
		require.NoError(t, err)
		require.Len(t, got, 1)
		require.IsType(t, starter.UnixListener{}, got[0])
		require.Equal(t, starter.NewUnixListener("unix.sock", 5), got[0])
	})

	t.Run("unix path containing colon and digits is not read as TCP", func(t *testing.T) {
		got, err := starter.ParsePorts("/tmp/a:80=5")
		require.NoError(t, err)
		require.Len(t, got, 1)
		require.IsType(t, starter.UnixListener{}, got[0])
		require.Equal(t, starter.NewUnixListener("/tmp/a:80", 5), got[0])
	})

	t.Run("dot slash prefixed numeric path is a unix socket", func(t *testing.T) {
		got, err := starter.ParsePorts("./8080=5")
		require.NoError(t, err)
		require.Len(t, got, 1)
		require.IsType(t, starter.UnixListener{}, got[0])
		require.Equal(t, starter.NewUnixListener("./8080", 5), got[0])
	})

	t.Run("mixed multi-target spec", func(t *testing.T) {
		got, err := starter.ParsePorts("unix.sock=5;udp://8080=3;10.0.0.5:9090=4;/foo/bar.sock=6")
		require.NoError(t, err)
		require.Len(t, got, 4)
		require.Equal(t, starter.NewUnixListener("unix.sock", 5), got[0])
		require.Equal(t, starter.NewUDPListener("", 8080, 3), got[1])
		require.Equal(t, starter.NewTCPListener("10.0.0.5", 9090, 4), got[2])
		require.Equal(t, starter.NewUnixListener("/foo/bar.sock", 6), got[3])
	})

	t.Run("port boundaries", func(t *testing.T) {
		for _, spec := range []string{"0=3", "65535=3"} {
			got, err := starter.ParsePorts(spec)
			require.NoError(t, err)
			require.Len(t, got, 1)
		}
	})

	t.Run("port above range", func(t *testing.T) {
		for _, spec := range []string{"65536=3", "99999=3", "127.0.0.1:65536=3"} {
			got, err := starter.ParsePorts(spec)
			require.EqualError(t, err, fmt.Sprintf("invalid port in %q", spec))
			require.Nil(t, got)
		}
	})

	t.Run("descriptor overlaps standard streams", func(t *testing.T) {
		for _, spec := range []string{"8080=0", "8080=1", "8080=2"} {
			got, err := starter.ParsePorts(spec)
			require.EqualError(t, err,
				fmt.Sprintf("failed to parse '%s' as listen target: file descriptor must be at least 3", spec))
			require.Nil(t, got)
		}
	})
}

func TestFormatPorts(t *testing.T) {
	t.Run("rejects an empty List", func(t *testing.T) {
		_, err := starter.FormatPorts(starter.List{}...)
		require.ErrorIs(t, err, starter.ErrNoListeningTarget)
	})

	t.Run("rejects empty TCP Addr", func(t *testing.T) {
		_, err := starter.FormatPorts(starter.TCPListener{Addr: "", Port: 8080})
		require.Error(t, err)
		require.ErrorContains(t, err, "TCPListener")
		require.ErrorContains(t, err, addrField)
	})

	t.Run("rejects empty UDP Addr", func(t *testing.T) {
		_, err := starter.FormatPorts(starter.UDPListener{Addr: "", Port: 8080})
		require.Error(t, err)
		require.ErrorContains(t, err, "UDPListener")
		require.ErrorContains(t, err, addrField)
	})

	t.Run("rejects empty unix Path", func(t *testing.T) {
		_, err := starter.FormatPorts(starter.UnixListener{Path: ""})
		require.Error(t, err)
		require.ErrorContains(t, err, "UnixListener")
		require.ErrorContains(t, err, "Path")
	})

	t.Run("rejects NUL bytes", func(t *testing.T) {
		tests := []struct {
			name     string
			listener starter.Listener
			field    string
		}{
			{
				name:     "TCP Addr",
				listener: starter.NewTCPListener("127.0.0.1\x00bad", 8080, 3),
				field:    addrField,
			},
			{
				name:     "UDP Addr",
				listener: starter.NewUDPListener("127.0.0.1\x00bad", 8080, 3),
				field:    addrField,
			},
			{
				name:     "unix Path",
				listener: starter.NewUnixListener("/tmp/app\x00bad.sock", 3),
				field:    "Path",
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				_, err := starter.FormatPorts(test.listener)
				require.Error(t, err)
				require.ErrorContains(t, err, test.field)
				require.ErrorContains(t, err, "NUL")
			})
		}
	})

	t.Run("rejects reserved wire delimiters", func(t *testing.T) {
		tests := []struct {
			name      string
			listener  starter.Listener
			field     string
			delimiter string
		}{
			{
				name:      "TCP Addr semicolon",
				listener:  starter.NewTCPListener("host;next", 8080, 3),
				field:     "Addr",
				delimiter: ";",
			},
			{
				name:      "TCP Addr equals",
				listener:  starter.NewTCPListener("host=next", 8080, 3),
				field:     "Addr",
				delimiter: "=",
			},
			{
				name:      "UDP Addr semicolon",
				listener:  starter.NewUDPListener("host;next", 8080, 3),
				field:     "Addr",
				delimiter: ";",
			},
			{
				name:      "UDP Addr equals",
				listener:  starter.NewUDPListener("host=next", 8080, 3),
				field:     "Addr",
				delimiter: "=",
			},
			{
				name:      "Unix Path semicolon",
				listener:  starter.NewUnixListener("/tmp/app;next.sock", 3),
				field:     "Path",
				delimiter: ";",
			},
			{
				name:      "Unix Path equals",
				listener:  starter.NewUnixListener("/tmp/app=next.sock", 3),
				field:     "Path",
				delimiter: "=",
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				spec, err := starter.FormatPorts(test.listener)
				require.Error(t, err)
				require.Empty(t, spec)
				require.ErrorContains(t, err, test.field)
				require.ErrorContains(t, err, test.delimiter)
			})
		}
	})

	t.Run("rejects an unknown Listener implementation", func(t *testing.T) {
		_, err := starter.FormatPorts(fakeListener{})
		require.Error(t, err)
	})

	t.Run("rejects an ambiguous relative unix socket path", func(t *testing.T) {
		_, err := starter.FormatPorts(starter.NewUnixListener("8080", 3))
		require.Error(t, err)
		require.ErrorContains(t, err, "UnixListener")
		require.ErrorContains(t, err, "TCPListener")
	})

	t.Run("accepts a TCP address beginning with u", func(t *testing.T) {
		spec, err := starter.FormatPorts(starter.NewTCPListener("upstream", 8080, 3))
		require.NoError(t, err)
		require.Equal(t, "upstream:8080=3", spec)
	})

	// Round-trip coverage against ParsePorts for every valid shape the
	// supervisor can emit (bare port, host:port, bracketed IPv6, unix
	// path, and the UDP variants) lives in
	// TestListFormatPortsParsePortsRoundTrip in listener_test.go.

	t.Run("formats a List via variadic unpacking", func(t *testing.T) {
		list := starter.List{
			starter.NewTCPListener("127.0.0.1", 9090, 4),
			starter.NewUnixListener("/tmp/app.sock", 5),
		}
		spec, err := starter.FormatPorts(list...)
		require.NoError(t, err)
		require.Equal(t, "127.0.0.1:9090=4;/tmp/app.sock=5", spec)
	})
}

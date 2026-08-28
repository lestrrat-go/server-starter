package starter_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	starter "github.com/lestrrat-go/server-starter/v2"
)

func TestTCPListenerString(t *testing.T) {
	t.Run("wildcard address", func(t *testing.T) {
		l := starter.NewTCPListener("0.0.0.0", 8080, 3)
		require.Equal(t, "8080=3", l.String())
	})

	t.Run("specific address", func(t *testing.T) {
		l := starter.NewTCPListener("10.0.0.5", 9090, 4)
		require.Equal(t, "10.0.0.5:9090=4", l.String())
	})

	t.Run("ipv6 address", func(t *testing.T) {
		l := starter.NewTCPListener("::1", 9090, 4)
		require.Equal(t, "[::1]:9090=4", l.String())
	})
}

func TestTCPListenerFd(t *testing.T) {
	l := starter.NewTCPListener("10.0.0.5", 9090, 42)
	require.Equal(t, uintptr(42), l.Fd())
}

func TestUDPListenerString(t *testing.T) {
	t.Run("wildcard address", func(t *testing.T) {
		l := starter.NewUDPListener("0.0.0.0", 8080, 3)
		require.Equal(t, "u8080=3", l.String())
	})

	t.Run("specific address", func(t *testing.T) {
		l := starter.NewUDPListener("192.168.1.20", 9092, 5)
		require.Equal(t, "u192.168.1.20:9092=5", l.String())
	})

	t.Run("ipv6 address", func(t *testing.T) {
		l := starter.NewUDPListener("2001:db8::1", 9093, 6)
		require.Equal(t, "u[2001:db8::1]:9093=6", l.String())
	})
}

func TestUDPListenerFd(t *testing.T) {
	l := starter.NewUDPListener("192.168.1.20", 9092, 43)
	require.Equal(t, uintptr(43), l.Fd())
}

func TestUnixListenerString(t *testing.T) {
	l := starter.NewUnixListener("/var/run/app.sock", 7)
	require.Equal(t, "/var/run/app.sock=7", l.String())
}

func TestUnixListenerFd(t *testing.T) {
	l := starter.NewUnixListener("/var/run/app.sock", 44)
	require.Equal(t, uintptr(44), l.Fd())
}

func TestNewTCPListenerNormalisesEmptyAddr(t *testing.T) {
	l := starter.NewTCPListener("", 8080, 1)
	require.Equal(t, "0.0.0.0", l.Addr)
	require.Equal(t, "8080=1", l.String())
}

func TestNewUDPListenerNormalisesEmptyAddr(t *testing.T) {
	l := starter.NewUDPListener("", 8080, 1)
	require.Equal(t, "0.0.0.0", l.Addr)
	require.Equal(t, "u8080=1", l.String())
}

func TestNewUnixListenerNoNormalisation(t *testing.T) {
	l := starter.NewUnixListener("relative.sock", 1)
	require.Equal(t, "relative.sock", l.Path)
	require.Equal(t, "relative.sock=1", l.String())
}

func TestListString(t *testing.T) {
	list := starter.List{
		starter.NewTCPListener("10.0.0.5", 9090, 3),
		starter.NewUDPListener("192.168.1.20", 9092, 4),
		starter.NewUnixListener("/var/run/app.sock", 5),
	}
	require.Equal(t, "10.0.0.5:9090=3;u192.168.1.20:9092=4;/var/run/app.sock=5", list.String())
}

// TestListStringParsePortsRoundTrip checks that List.String() and ParsePorts
// are inverses of each other, scoped to the shapes the supervisor can
// actually emit: a bare TCP port, "host:port", "[ipv6]:port" (each with and
// without the UDP "u" prefix), and an absolute unix socket path.
//
// A relative unix socket path with no "/" that happens to parse as a port or
// "host:port" (e.g. "8080" or "db:5432") is deliberately excluded because
// ParsePorts reads it back as TCP/UDP, per the documented ambiguity on
// ParsePorts.
func TestListStringParsePortsRoundTrip(t *testing.T) {
	t.Run("empty list", func(t *testing.T) {
		list := starter.List{}
		got, err := starter.ParsePorts(list.String())
		require.NoError(t, err)
		require.Equal(t, list, got)
	})

	t.Run("bare TCP port", func(t *testing.T) {
		list := starter.List{starter.NewTCPListener("", 8080, 3)}
		got, err := starter.ParsePorts(list.String())
		require.NoError(t, err)
		require.Equal(t, list, got)
	})

	t.Run("TCP host and port", func(t *testing.T) {
		list := starter.List{starter.NewTCPListener("10.0.0.5", 9090, 4)}
		got, err := starter.ParsePorts(list.String())
		require.NoError(t, err)
		require.Equal(t, list, got)
	})

	t.Run("TCP ipv6 host and port", func(t *testing.T) {
		list := starter.List{starter.NewTCPListener("::1", 9090, 5)}
		got, err := starter.ParsePorts(list.String())
		require.NoError(t, err)
		require.Equal(t, list, got)
	})

	t.Run("bare UDP port", func(t *testing.T) {
		list := starter.List{starter.NewUDPListener("", 8080, 6)}
		got, err := starter.ParsePorts(list.String())
		require.NoError(t, err)
		require.Equal(t, list, got)
	})

	t.Run("UDP host and port", func(t *testing.T) {
		list := starter.List{starter.NewUDPListener("192.168.1.20", 9092, 7)}
		got, err := starter.ParsePorts(list.String())
		require.NoError(t, err)
		require.Equal(t, list, got)
	})

	t.Run("UDP ipv6 host and port", func(t *testing.T) {
		list := starter.List{starter.NewUDPListener("2001:db8::1", 9093, 8)}
		got, err := starter.ParsePorts(list.String())
		require.NoError(t, err)
		require.Equal(t, list, got)
	})

	t.Run("absolute unix socket path", func(t *testing.T) {
		list := starter.List{starter.NewUnixListener("/var/run/app.sock", 9)}
		got, err := starter.ParsePorts(list.String())
		require.NoError(t, err)
		require.Equal(t, list, got)
	})

	t.Run("mixed multi-target list", func(t *testing.T) {
		list := starter.List{
			starter.NewTCPListener("", 8080, 3),
			starter.NewTCPListener("10.0.0.5", 9090, 4),
			starter.NewTCPListener("::1", 9091, 5),
			starter.NewUDPListener("", 8081, 6),
			starter.NewUDPListener("192.168.1.20", 9092, 7),
			starter.NewUDPListener("2001:db8::1", 9093, 8),
			starter.NewUnixListener("/var/run/app.sock", 9),
		}
		got, err := starter.ParsePorts(list.String())
		require.NoError(t, err)
		require.Equal(t, list, got)
	})
}

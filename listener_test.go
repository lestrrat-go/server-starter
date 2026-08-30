package starter_test

import (
	"path/filepath"
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
		require.Equal(t, "8080=3", l.String())
	})

	t.Run("specific address", func(t *testing.T) {
		l := starter.NewUDPListener("192.168.1.20", 9092, 5)
		require.Equal(t, "192.168.1.20:9092=5", l.String())
	})

	t.Run("ipv6 address", func(t *testing.T) {
		l := starter.NewUDPListener("2001:db8::1", 9093, 6)
		require.Equal(t, "[2001:db8::1]:9093=6", l.String())
	})

	t.Run("hostname", func(t *testing.T) {
		l := starter.NewUDPListener("upstream", 9094, 7)
		require.Equal(t, "upstream:9094=7", l.String())
	})
}

func TestUDPListenerFd(t *testing.T) {
	l := starter.NewUDPListener("192.168.1.20", 9092, 43)
	require.Equal(t, uintptr(43), l.Fd())
}

func TestUnixListenerString(t *testing.T) {
	t.Run("absolute path", func(t *testing.T) {
		l := starter.NewUnixListener("/var/run/app.sock", 7)
		require.Equal(t, "/var/run/app.sock=7", l.String())
	})

	t.Run("ambiguous struct literal", func(t *testing.T) {
		l := starter.UnixListener{Path: "8080"}
		require.Equal(t, "./8080=0", l.String())
	})
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
	require.Equal(t, "8080=1", l.String())
}

func TestNewUnixListenerCanonicalPath(t *testing.T) {
	tests := []struct {
		name string
		path string
		want string
	}{
		{name: "ordinary relative path", path: "relative.sock", want: "relative.sock"},
		{name: "relative path with slash", path: "dir/8080", want: "dir/8080"},
		{name: "absolute path", path: "/var/run/app.sock", want: "/var/run/app.sock"},
		{name: "already disambiguated path", path: "./8080", want: "./8080"},
		{name: "bare port grammar", path: "8080", want: "./8080"},
		{name: "host and port grammar", path: "db:5432", want: "./db:5432"},
		{name: "multi-colon host and port", path: "a:b:8080", want: "./a:b:8080"},
		{name: "TCP hostname beginning with u", path: "ubuntu.internal:8080", want: "./ubuntu.internal:8080"},
		{name: "path beginning with udp", path: "udp://8080", want: "udp://8080"},
		{name: "relative path beginning with udp", path: "udp://relative.sock", want: "udp://relative.sock"},
		{name: "leading UDP grammar", path: "u8080", want: "./u8080"},
		{name: "trailing UDP grammar", path: "db:u5432", want: "./db:u5432"},
		{name: "leading space before bare port", path: " 8080", want: "./ 8080"},
		{name: "trailing space after bare port", path: "8080 ", want: "./8080 "},
		{name: "leading space before udp path", path: " udp://8080", want: " udp://8080"},
		{name: "leading space before legacy UDP grammar", path: " u8080", want: "./ u8080"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			l := starter.NewUnixListener(tc.path, 1)
			require.Equal(t, tc.want, l.Path)
			require.Equal(t, tc.want+"=1", l.String())
			require.Equal(t, filepath.Clean(tc.path), filepath.Clean(l.Path))
		})
	}
}

func TestListFormatPorts(t *testing.T) {
	list := starter.List{
		starter.NewTCPListener("10.0.0.5", 9090, 3),
		starter.NewUDPListener("192.168.1.20", 9092, 4),
		starter.NewUnixListener("/var/run/app.sock", 5),
	}
	spec, err := starter.FormatPorts(list...)
	require.NoError(t, err)
	require.Equal(t, "10.0.0.5:9090=3;192.168.1.20:9092=4;/var/run/app.sock=5", spec)
	types, err := starter.FormatSocketTypes(list...)
	require.NoError(t, err)
	require.Equal(t, "3=tcp;4=udp;5=unix", types)
}

func TestListStringCompatibility(t *testing.T) {
	list := starter.List{
		starter.NewTCPListener("10.0.0.5", 9090, 3),
		starter.NewUDPListener("192.168.1.20", 9092, 4),
		starter.NewUnixListener("/var/run/app.sock", 5),
	}
	require.Equal(t, "10.0.0.5:9090=3;192.168.1.20:9092=4;/var/run/app.sock=5", list.String())
}

// TestListFormatPortsPortsRoundTrip checks that the two listener protocol
// variables preserve every listener type the v2 supervisor can emit.
func TestListFormatPortsPortsRoundTrip(t *testing.T) {
	roundTrip := func(t *testing.T, list starter.List) {
		t.Helper()
		spec, err := starter.FormatPorts(list...)
		require.NoError(t, err)
		types, err := starter.FormatSocketTypes(list...)
		require.NoError(t, err)
		if len(list) == 0 {
			got, err := starter.ParsePorts(spec)
			require.NoError(t, err)
			require.Equal(t, list, got)
			return
		}
		t.Setenv(starter.PortEnvName, spec)
		t.Setenv(starter.SocketTypesEnvName, types)
		got, err := starter.Ports()
		require.NoError(t, err)
		require.Equal(t, list, got)
	}

	t.Run("empty list", func(t *testing.T) {
		var list starter.List
		roundTrip(t, list)
	})

	t.Run("bare TCP port", func(t *testing.T) {
		roundTrip(t, starter.List{starter.NewTCPListener("", 8080, 3)})
	})

	t.Run("TCP host and port", func(t *testing.T) {
		roundTrip(t, starter.List{starter.NewTCPListener("10.0.0.5", 9090, 4)})
	})

	t.Run("TCP ipv6 host and port", func(t *testing.T) {
		roundTrip(t, starter.List{starter.NewTCPListener("::1", 9090, 5)})
	})

	t.Run("bare UDP port", func(t *testing.T) {
		roundTrip(t, starter.List{starter.NewUDPListener("", 8080, 6)})
	})

	t.Run("UDP host and port", func(t *testing.T) {
		roundTrip(t, starter.List{starter.NewUDPListener("192.168.1.20", 9092, 7)})
	})

	t.Run("UDP hostname and port", func(t *testing.T) {
		roundTrip(t, starter.List{starter.NewUDPListener("upstream", 9092, 7)})
	})

	t.Run("UDP ipv6 host and port", func(t *testing.T) {
		roundTrip(t, starter.List{starter.NewUDPListener("2001:db8::1", 9093, 8)})
	})

	t.Run("absolute unix socket path", func(t *testing.T) {
		roundTrip(t, starter.List{starter.NewUnixListener("/var/run/app.sock", 9)})
	})

	t.Run("grammar-ambiguous unix socket paths", func(t *testing.T) {
		for _, path := range []string{
			"8080", "db:5432", "a:b:8080", "ubuntu.internal:8080", "u8080",
			"db:u5432", "ubuntu.internal:u8080", " 8080", " [::1]:5432", " u8080", "8080 ",
		} {
			list := starter.List{starter.NewUnixListener(path, 9)}
			roundTrip(t, list)
		}
	})

	t.Run("mixed multi-target list", func(t *testing.T) {
		roundTrip(t, starter.List{
			starter.NewTCPListener("", 8080, 3),
			starter.NewTCPListener("10.0.0.5", 9090, 4),
			starter.NewTCPListener("::1", 9091, 5),
			starter.NewUDPListener("", 8081, 6),
			starter.NewUDPListener("192.168.1.20", 9092, 7),
			starter.NewUDPListener("2001:db8::1", 9093, 8),
			starter.NewUnixListener("/var/run/app.sock", 9),
			starter.NewUnixListener("8080", 10),
		})
	})
}

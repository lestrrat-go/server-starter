package listener

import (
	"testing"

	"github.com/stretchr/testify/require"
)

const loopbackAddr = "127.0.0.1"

func TestPort(t *testing.T) {
	expect := List{
		TCPListener{Addr: loopbackAddr, Port: 9090, fd: 4},
		TCPListener{Addr: wildcardIPv4, Port: 8080, fd: 5},
		UnixListener{Path: "/foo/bar/baz.sock", fd: 6},
	}

	t.Setenv("SERVER_STARTER_PORT", expect.String())
	ports, err := Ports()
	if err != nil {
		t.Errorf("Failed to parse ports from env: %s", err)
	}

	for i, port := range ports {
		if port.Fd() != expect[i].Fd() {
			t.Errorf("parsed fd is not what we expected (expected %d, got %d)", expect[i].Fd(), port.Fd())
		}
		_, gotTCP := port.(TCPListener)
		_, expectTCP := expect[i].(TCPListener)
		if gotTCP != expectTCP {
			t.Errorf("parsed listener is the wrong type")
		}
	}
}

func TestParseListenTargets(t *testing.T) {
	t.Run("bare port", func(t *testing.T) {
		got, err := parseListenTargets("8080=3")
		require.NoError(t, err)
		require.Len(t, got, 1)
		require.IsType(t, TCPListener{}, got[0])
		require.Equal(t, TCPListener{Addr: "0.0.0.0", Port: 8080, fd: 3}, got[0])
	})

	t.Run("host and port", func(t *testing.T) {
		got, err := parseListenTargets("127.0.0.1:9090=4")
		require.NoError(t, err)
		require.Len(t, got, 1)
		require.IsType(t, TCPListener{}, got[0])
		require.Equal(t, TCPListener{Addr: loopbackAddr, Port: 9090, fd: 4}, got[0])
	})

	t.Run("IPv6 host and port", func(t *testing.T) {
		got, err := parseListenTargets("[::1]:9090=4")
		require.NoError(t, err)
		require.Equal(t, TCPListener{Addr: "::1", Port: 9090, fd: 4}, got[0])
		require.Equal(t, "[::1]:9090=4", got[0].String())
	})

	t.Run("UDP host and port", func(t *testing.T) {
		got, err := parseListenTargets("u127.0.0.1:9090=4")
		require.NoError(t, err)
		require.Equal(t, UDPListener{Addr: loopbackAddr, Port: 9090, fd: 4}, got[0])
	})

	t.Run("UDP port suffix", func(t *testing.T) {
		got, err := parseListenTargets("127.0.0.1:u9090=4")
		require.NoError(t, err)
		require.Equal(t, UDPListener{Addr: loopbackAddr, Port: 9090, fd: 4}, got[0])
	})

	t.Run("UDP IPv6 host and port", func(t *testing.T) {
		got, err := parseListenTargets("u[::1]:9090=4")
		require.NoError(t, err)
		require.Equal(t, UDPListener{Addr: "::1", Port: 9090, fd: 4}, got[0])
		require.Equal(t, "u[::1]:9090=4", got[0].String())
	})

	t.Run("unix socket path", func(t *testing.T) {
		got, err := parseListenTargets("/foo/bar.sock=5")
		require.NoError(t, err)
		require.Len(t, got, 1)
		require.IsType(t, UnixListener{}, got[0])
		require.Equal(t, UnixListener{Path: "/foo/bar.sock", fd: 5}, got[0])
	})

	t.Run("missing equals sign", func(t *testing.T) {
		got, err := parseListenTargets("8080")
		require.Error(t, err)
		require.Nil(t, got)
	})

	t.Run("multiple equals signs", func(t *testing.T) {
		got, err := parseListenTargets("8080=3=extra")
		require.Error(t, err)
		require.Nil(t, got)
	})

	t.Run("empty string", func(t *testing.T) {
		got, err := parseListenTargets("")
		require.ErrorIs(t, err, ErrNoListeningTarget)
		require.Nil(t, got)
	})

	t.Run("relative unix path starting with u is not stripped", func(t *testing.T) {
		got, err := parseListenTargets("unix.sock=5")
		require.NoError(t, err)
		require.Len(t, got, 1)
		require.IsType(t, UnixListener{}, got[0])
		require.Equal(t, UnixListener{Path: "unix.sock", fd: 5}, got[0])
	})

	t.Run("unix path containing colon and digits is not read as TCP", func(t *testing.T) {
		got, err := parseListenTargets("/tmp/a:80=5")
		require.NoError(t, err)
		require.Len(t, got, 1)
		require.IsType(t, UnixListener{}, got[0])
		require.Equal(t, UnixListener{Path: "/tmp/a:80", fd: 5}, got[0])
	})

	t.Run("dot slash prefixed numeric path is a unix socket", func(t *testing.T) {
		got, err := parseListenTargets("./8080=5")
		require.NoError(t, err)
		require.Len(t, got, 1)
		require.IsType(t, UnixListener{}, got[0])
		require.Equal(t, UnixListener{Path: "./8080", fd: 5}, got[0])
	})

	t.Run("mixed multi-target spec", func(t *testing.T) {
		got, err := parseListenTargets("unix.sock=5;u8080=3;127.0.0.1:9090=4;/foo/bar.sock=6")
		require.NoError(t, err)
		require.Len(t, got, 4)
		require.Equal(t, UnixListener{Path: "unix.sock", fd: 5}, got[0])
		require.Equal(t, UDPListener{Addr: wildcardIPv4, Port: 8080, fd: 3}, got[1])
		require.Equal(t, TCPListener{Addr: loopbackAddr, Port: 9090, fd: 4}, got[2])
		require.Equal(t, UnixListener{Path: "/foo/bar.sock", fd: 6}, got[3])
	})
}

func TestPortNoEnv(t *testing.T) {
	t.Setenv("SERVER_STARTER_PORT", "")

	ports, err := Ports()
	if err != ErrNoListeningTarget {
		t.Error("Ports must return error if no env")
	}

	if ports != nil {
		t.Errorf("Ports must return nil if no env")
	}
}

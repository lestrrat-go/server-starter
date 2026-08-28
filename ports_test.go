package starter_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	starter "github.com/lestrrat-go/server-starter/v2"
)

func TestPorts(t *testing.T) {
	expect := starter.List{
		starter.NewTCPListener("127.0.0.1", 9090, 4),
		starter.NewTCPListener("", 8080, 5),
		starter.NewUnixListener("/foo/bar/baz.sock", 6),
	}

	t.Setenv(starter.PortEnvName, expect.String())
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
		got, err := starter.ParsePorts("u127.0.0.1:9090=4")
		require.NoError(t, err)
		require.Equal(t, starter.NewUDPListener("127.0.0.1", 9090, 4), got[0])
	})

	t.Run("UDP port suffix", func(t *testing.T) {
		got, err := starter.ParsePorts("127.0.0.1:u9090=4")
		require.NoError(t, err)
		require.Equal(t, starter.NewUDPListener("127.0.0.1", 9090, 4), got[0])
	})

	t.Run("UDP IPv6 host and port", func(t *testing.T) {
		got, err := starter.ParsePorts("u[::1]:9090=4")
		require.NoError(t, err)
		require.Equal(t, starter.NewUDPListener("::1", 9090, 4), got[0])
		require.Equal(t, "u[::1]:9090=4", got[0].String())
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
		require.NoError(t, err)
		require.Equal(t, starter.List{}, got)
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
		got, err := starter.ParsePorts("unix.sock=5;u8080=3;10.0.0.5:9090=4;/foo/bar.sock=6")
		require.NoError(t, err)
		require.Len(t, got, 4)
		require.Equal(t, starter.NewUnixListener("unix.sock", 5), got[0])
		require.Equal(t, starter.NewUDPListener("", 8080, 3), got[1])
		require.Equal(t, starter.NewTCPListener("10.0.0.5", 9090, 4), got[2])
		require.Equal(t, starter.NewUnixListener("/foo/bar.sock", 6), got[3])
	})
}

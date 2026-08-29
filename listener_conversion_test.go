//go:build !windows

package starter_test

import (
	"net"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	starter "github.com/lestrrat-go/server-starter/v2"
)

func TestListenerConversionClosesSourceFile(t *testing.T) {
	t.Run("TCP", func(t *testing.T) {
		source, err := net.ListenTCP("tcp", &net.TCPAddr{IP: net.ParseIP("127.0.0.1")})
		require.NoError(t, err)

		file, err := source.File()
		require.NoError(t, err)
		t.Cleanup(func() { _ = file.Close() })
		address := source.Addr().(*net.TCPAddr)
		require.NoError(t, source.Close())

		listener, err := starter.NewTCPListener(address.IP.String(), address.Port, file.Fd()).Listen()
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, listener.Close()) })
		require.Error(t, file.Close())
	})

	t.Run("UDP", func(t *testing.T) {
		source, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
		require.NoError(t, err)

		file, err := source.File()
		require.NoError(t, err)
		t.Cleanup(func() { _ = file.Close() })
		address := source.LocalAddr().(*net.UDPAddr)
		require.NoError(t, source.Close())

		packetConn, err := starter.NewUDPListener(address.IP.String(), address.Port, file.Fd()).ListenPacket()
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, packetConn.Close()) })
		require.Error(t, file.Close())
	})

	t.Run("unix", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "listener.sock")
		source, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
		require.NoError(t, err)

		file, err := source.File()
		require.NoError(t, err)
		t.Cleanup(func() { _ = file.Close() })
		require.NoError(t, source.Close())

		listener, err := starter.NewUnixListener(path, file.Fd()).Listen()
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, listener.Close()) })
		require.Error(t, file.Close())
	})
}

func TestListenerConversionRejectsInvalidFileDescriptor(t *testing.T) {
	invalidFD := ^uintptr(0)

	listener, err := starter.NewTCPListener("127.0.0.1", 8080, invalidFD).Listen()
	require.Nil(t, listener)
	require.ErrorContains(t, err, "invalid TCP listener file descriptor")

	packetConn, err := starter.NewUDPListener("127.0.0.1", 8080, invalidFD).ListenPacket()
	require.Nil(t, packetConn)
	require.ErrorContains(t, err, "invalid UDP listener file descriptor")

	listener, err = starter.NewUnixListener("listener.sock", invalidFD).Listen()
	require.Nil(t, listener)
	require.ErrorContains(t, err, "invalid unix listener file descriptor")
}

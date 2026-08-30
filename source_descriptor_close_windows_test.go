package starter_test

import (
	"net"
	"testing"

	"github.com/stretchr/testify/require"

	starter "github.com/lestrrat-go/server-starter/v2"
)

func TestWindowsListenerConversionClosesSourceSocket(t *testing.T) {
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
}

package starter

import (
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
)

const wildcardIPv4 = "0.0.0.0"

// ErrNoListeningTarget is returned by Ports when SERVER_STARTER_PORT is empty
// or unset.
var ErrNoListeningTarget = errors.New("starter: no listening target")

// Listener is the interface for things that listen on file descriptors
// specified by Start::Server / server_starter
type Listener interface {
	Fd() uintptr
	Listen() (net.Listener, error)
	String() string
}

// List holds a list of Listeners. This is here just for convenience
// so that you can do
//
//	list.String()
//
// to get a string compatible with SERVER_STARTER_PORT
type List []Listener

// String joins every Listener's String() with ";", producing a value
// compatible with SERVER_STARTER_PORT.
func (ll List) String() string {
	list := make([]string, len(ll))
	for i, l := range ll {
		list[i] = l.String()
	}
	return strings.Join(list, ";")
}

// TCPListener is a listener for ... tcp duh.
type TCPListener struct {
	Addr string
	Port int
	fd   uintptr
}

// NewTCPListener creates a TCPListener for addr, port, and the inherited
// file descriptor fd. An empty addr is normalised to the IPv4 wildcard
// address ("0.0.0.0") so the stored value round-trips correctly through
// String and ParsePorts; without this, TCPListener{Addr: ""}.String() would
// produce ":8080", which ParsePorts reads back as a unix socket at path
// ":8080".
func NewTCPListener(addr string, port int, fd uintptr) TCPListener {
	if addr == "" {
		addr = wildcardIPv4
	}
	return TCPListener{Addr: addr, Port: port, fd: fd}
}

// UDPListener is a UDP endpoint passed through SERVER_STARTER_PORT.
type UDPListener struct {
	Addr string
	Port int
	fd   uintptr
}

// NewUDPListener creates a UDPListener for addr, port, and the inherited
// file descriptor fd. An empty addr is normalised to the IPv4 wildcard
// address ("0.0.0.0"), for the same reason as NewTCPListener.
func NewUDPListener(addr string, port int, fd uintptr) UDPListener {
	if addr == "" {
		addr = wildcardIPv4
	}
	return UDPListener{Addr: addr, Port: port, fd: fd}
}

// UnixListener is a listener for unix sockets.
type UnixListener struct {
	Path string
	fd   uintptr
}

// NewUnixListener creates a UnixListener for path and the inherited file
// descriptor fd. path is taken verbatim, with no normalisation.
func NewUnixListener(path string, fd uintptr) UnixListener {
	return UnixListener{Path: path, fd: fd}
}

// String returns l in the "spec=fd" form used by SERVER_STARTER_PORT.
func (l TCPListener) String() string {
	if l.Addr == wildcardIPv4 {
		return fmt.Sprintf("%d=%d", l.Port, l.fd)
	}
	return fmt.Sprintf("%s=%d", net.JoinHostPort(l.Addr, strconv.Itoa(l.Port)), l.fd)
}

// Fd returns the underlying file descriptor
func (l TCPListener) Fd() uintptr {
	return l.fd
}

// Listen creates a new Listener
func (l TCPListener) Listen() (net.Listener, error) {
	return net.FileListener(os.NewFile(l.Fd(), net.JoinHostPort(l.Addr, strconv.Itoa(l.Port))))
}

// String returns l in the "uspec=fd" form used by SERVER_STARTER_PORT.
func (l UDPListener) String() string {
	address := strconv.Itoa(l.Port)
	if l.Addr != wildcardIPv4 {
		address = net.JoinHostPort(l.Addr, strconv.Itoa(l.Port))
	}
	return fmt.Sprintf("u%s=%d", address, l.fd)
}

// Fd returns the underlying file descriptor.
func (l UDPListener) Fd() uintptr {
	return l.fd
}

// Listen returns an error because UDP endpoints are packet connections.
func (l UDPListener) Listen() (net.Listener, error) {
	return nil, fmt.Errorf("UDP listener requires ListenPacket")
}

// ListenPacket creates a packet connection from the inherited descriptor.
func (l UDPListener) ListenPacket() (net.PacketConn, error) {
	return net.FilePacketConn(os.NewFile(l.Fd(), net.JoinHostPort(l.Addr, strconv.Itoa(l.Port))))
}

// String returns l in the "path=fd" form used by SERVER_STARTER_PORT.
func (l UnixListener) String() string {
	return fmt.Sprintf("%s=%d", l.Path, l.fd)
}

// Fd returns the underlying file descriptor
func (l UnixListener) Fd() uintptr {
	return l.fd
}

// Listen creates a new Listener
func (l UnixListener) Listen() (net.Listener, error) {
	return net.FileListener(os.NewFile(l.Fd(), l.Path))
}

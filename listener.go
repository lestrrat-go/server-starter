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

// ErrNoListeningTarget is returned when there is nothing to listen on, for
// example when SERVER_STARTER_PORT is empty or unset.
var ErrNoListeningTarget = errors.New("starter: no listening target")

// Listener describes an endpoint inherited from Start::Server or
// server_starter. TCPListener and UnixListener create stream listeners with
// Listen. UDPListener implements Listen to satisfy this interface, but that
// method returns an error; use UDPListener.ListenPacket for UDP endpoints.
type Listener interface {
	Fd() uintptr
	Listen() (net.Listener, error)
	String() string
}

// List holds a list of Listeners.
type List []Listener

// String joins every Listener's display form with ";". It is retained for
// compatibility with v0 and earlier v2 releases. Use FormatPorts when the
// result will be passed through SERVER_STARTER_PORT, because String does not
// validate that the result is safe for an environment variable or that
// ParsePorts can read it back as the same listeners.
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
// ":8080". FormatPorts rejects addr values containing the reserved wire
// delimiters ';' or '='.
func NewTCPListener(addr string, port int, fd uintptr) TCPListener {
	if addr == "" {
		addr = wildcardIPv4
	}
	return TCPListener{Addr: addr, Port: port, fd: fd}
}

// UDPListener is a UDP endpoint passed through SERVER_STARTER_PORT. Its type
// is carried separately in SocketTypesEnvName, because Perl's wire format
// represents UDP and TCP sockets identically. Create its packet connection
// with ListenPacket rather than Listen.
type UDPListener struct {
	Addr string
	Port int
	fd   uintptr
}

// NewUDPListener creates a UDPListener for addr, port, and the inherited
// file descriptor fd. An empty addr is normalised to the IPv4 wildcard
// address ("0.0.0.0"), for the same reason as NewTCPListener. FormatPorts
// rejects addr values containing the reserved wire delimiters ';' or '='.
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
// descriptor fd. Relative paths that ParsePorts would otherwise classify as
// TCP or UDP are prefixed with "./" so their type and filesystem location
// survive a wire-format round trip. FormatPorts rejects path values containing
// the reserved wire delimiters ';' or '='.
func NewUnixListener(path string, fd uintptr) UnixListener {
	return UnixListener{Path: canonicalUnixPath(path), fd: fd}
}

// String returns a human-readable "spec=fd" rendering of l. It is a
// display form: it does not validate l, so it can render an unnormalised
// listener (for example one built directly as a struct literal, bypassing
// NewTCPListener), or an Addr containing ';' or '=', into a spec that reads
// back as something else. For the authoritative SERVER_STARTER_PORT encoder,
// see FormatPorts.
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
	fd := l.Fd()
	file := os.NewFile(fd, net.JoinHostPort(l.Addr, strconv.Itoa(l.Port)))
	if file == nil {
		return nil, fmt.Errorf("invalid TCP listener file descriptor %d", fd)
	}

	listener, err := net.FileListener(file)
	closeErr := closeSourceDescriptor(file, fd)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to create TCP listener from file descriptor %d: %w",
			fd,
			errors.Join(err, closeErr),
		)
	}
	if closeErr != nil {
		_ = listener.Close()
		return nil, fmt.Errorf(
			"failed to close TCP listener file descriptor %d after conversion: %w",
			fd,
			closeErr,
		)
	}
	return listener, nil
}

// String returns the Server::Starter-compatible "spec=fd" rendering of l.
// It does not encode the UDP type. FormatSocketTypes carries that information
// for workers started by this implementation.
func (l UDPListener) String() string {
	address := strconv.Itoa(l.Port)
	if l.Addr != wildcardIPv4 {
		address = net.JoinHostPort(l.Addr, strconv.Itoa(l.Port))
	}
	return fmt.Sprintf("%s=%d", address, l.fd)
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
	fd := l.Fd()
	file := os.NewFile(fd, net.JoinHostPort(l.Addr, strconv.Itoa(l.Port)))
	if file == nil {
		return nil, fmt.Errorf("invalid UDP listener file descriptor %d", fd)
	}

	packetConn, err := net.FilePacketConn(file)
	closeErr := closeSourceDescriptor(file, fd)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to create UDP listener from file descriptor %d: %w",
			fd,
			errors.Join(err, closeErr),
		)
	}
	if closeErr != nil {
		_ = packetConn.Close()
		return nil, fmt.Errorf(
			"failed to close UDP listener file descriptor %d after conversion: %w",
			fd,
			closeErr,
		)
	}
	return packetConn, nil
}

// String returns a human-readable "path=fd" rendering of l. It is a
// display form: it does not validate l, so it can render an empty Path, or
// one containing ';' or '=', into a spec that ParsePorts cannot read back
// correctly. It canonicalises ambiguous relative paths even when l was built
// as a struct literal. For the authoritative SERVER_STARTER_PORT encoder, see
// FormatPorts.
func (l UnixListener) String() string {
	return fmt.Sprintf("%s=%d", canonicalUnixPath(l.Path), l.fd)
}

// Fd returns the underlying file descriptor
func (l UnixListener) Fd() uintptr {
	return l.fd
}

// Listen creates a new Listener
func (l UnixListener) Listen() (net.Listener, error) {
	fd := l.Fd()
	file := os.NewFile(fd, l.Path)
	if file == nil {
		return nil, fmt.Errorf("invalid unix listener file descriptor %d", fd)
	}

	listener, err := net.FileListener(file)
	closeErr := closeSourceDescriptor(file, fd)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to create unix listener from file descriptor %d: %w",
			fd,
			errors.Join(err, closeErr),
		)
	}
	if closeErr != nil {
		_ = listener.Close()
		return nil, fmt.Errorf(
			"failed to close unix listener file descriptor %d after conversion: %w",
			fd,
			closeErr,
		)
	}
	return listener, nil
}

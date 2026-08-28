package listener

import (
	"errors"
	"fmt"
	"net"
	"os"
	"regexp"
	"strconv"
	"strings"
)

// ServerStarterEnvVarName is the environment variable that carries the
// listener specification, as a list of "spec=fd" pairs joined by ";".
//
// Each spec is either a TCP/UDP target (a bare port, "host:port", or
// "[ipv6]:port", optionally prefixed with "u" for UDP) or a unix socket
// path. A path containing "/" is always read as a unix socket. A relative
// unix socket path with no "/" that happens to parse as a port or
// "host:port" (e.g. "8080" or "db:5432") is indistinguishable from a TCP/UDP
// spec in this wire format and is interpreted as TCP/UDP; pass such sockets
// as absolute paths, or prefix them with "./" (which contains "/" and so is
// always read as a unix socket).
const ServerStarterEnvVarName = "SERVER_STARTER_PORT"
const wildcardIPv4 = "0.0.0.0"

var (
	ErrNoListeningTarget = errors.New("no listening target")
)

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

// UDPListener is a UDP endpoint passed through SERVER_STARTER_PORT.
type UDPListener struct {
	Addr string
	Port int
	fd   uintptr
}

// UnixListener is a listener for unix sockets.
type UnixListener struct {
	Path string
	fd   uintptr
}

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

// Being lazy here...
var reLooksLikeHostPort = regexp.MustCompile(`^(.+?):(\d+)$`)
var reLooksLikePort = regexp.MustCompile(`^\d+$`)

// looksLikeTCPGrammar reports whether s parses as a bare port, "host:port",
// or "[ipv6]:port" — the grammar shared by TCP and UDP specs.
func looksLikeTCPGrammar(s string) bool {
	if reLooksLikeHostPort.MatchString(s) {
		return true
	}
	return reLooksLikePort.MatchString(s)
}

// parseListenTargets parses the "spec=fd;spec=fd;..." value carried by
// SERVER_STARTER_PORT (see ServerStarterEnvVarName) into concrete Listener
// values.
//
// Each spec is classified in this order: a spec containing "/" is always a
// unix socket path, taken verbatim; otherwise a spec beginning with "u"
// whose remainder parses as a port/host:port is a UDP target on that
// remainder; otherwise a spec that itself parses as a port/host:port is a
// TCP target; otherwise the spec is a unix socket path, taken verbatim.
//
// This leaves one shape ambiguous: a relative unix socket path with no "/"
// that happens to parse as a port or "host:port" (e.g. "8080" or "db:5432")
// is read as TCP, not as a unix socket. Pass such sockets as absolute
// paths, or prefix them with "./" to disambiguate.
func parseListenTargets(str string) ([]Listener, error) {
	if str == "" {
		return nil, ErrNoListeningTarget
	}

	rawspec := strings.Split(str, ";")
	ret := make([]Listener, len(rawspec))

	for i, pairString := range rawspec {
		pair := strings.SplitN(pairString, "=", 2)
		if len(pair) != 2 {
			return nil, fmt.Errorf("failed to parse '%s' as listen target: expected exactly one '='", pairString)
		}
		hostPort := strings.TrimSpace(pair[0])
		fdString := strings.TrimSpace(pair[1])
		fd, err := strconv.ParseUint(fdString, 10, 0)
		if err != nil {
			return nil, fmt.Errorf("failed to parse '%s' as listen target: %s", pairString, err)
		}

		if strings.ContainsRune(hostPort, '/') {
			ret[i] = UnixListener{Path: hostPort, fd: uintptr(fd)}
			continue
		}

		udp := false
		target := hostPort
		if candidate := strings.TrimPrefix(hostPort, "u"); candidate != hostPort && looksLikeTCPGrammar(candidate) {
			udp = true
			target = candidate
		} else if idx := strings.LastIndexByte(hostPort, ':'); idx >= 0 && strings.HasPrefix(hostPort[idx+1:], "u") {
			if candidate := hostPort[:idx+1] + strings.TrimPrefix(hostPort[idx+1:], "u"); looksLikeTCPGrammar(candidate) {
				udp = true
				target = candidate
			}
		}

		if matches := reLooksLikeHostPort.FindStringSubmatch(target); matches != nil {
			port, err := strconv.ParseInt(matches[2], 10, 0)
			if err != nil {
				return nil, err
			}

			if udp {
				ret[i] = UDPListener{Addr: strings.Trim(matches[1], "[]"), Port: int(port), fd: uintptr(fd)}
			} else {
				ret[i] = TCPListener{Addr: strings.Trim(matches[1], "[]"), Port: int(port), fd: uintptr(fd)}
			}
		} else if match := reLooksLikePort.FindString(target); match != "" {
			port, err := strconv.ParseInt(match, 10, 0)
			if err != nil {
				return nil, err
			}

			if udp {
				ret[i] = UDPListener{Addr: wildcardIPv4, Port: int(port), fd: uintptr(fd)}
			} else {
				ret[i] = TCPListener{Addr: wildcardIPv4, Port: int(port), fd: uintptr(fd)}
			}
		} else {
			ret[i] = UnixListener{
				Path: hostPort,
				fd:   uintptr(fd),
			}
		}
	}

	return ret, nil
}

// GetPortsSpecification returns the value of SERVER_STARTER_PORT
// environment variable
func GetPortsSpecification() string {
	return os.Getenv(ServerStarterEnvVarName)
}

// Ports parses environment variable SERVER_STARTER_PORT
func Ports() ([]Listener, error) {
	return parseListenTargets(GetPortsSpecification())
}

// ListenAll parses environment variable SERVER_STARTER_PORT, and creates
// net.Listener objects
func ListenAll() ([]net.Listener, error) {
	targets, err := parseListenTargets(GetPortsSpecification())
	if err != nil {
		return nil, err
	}

	ret := make([]net.Listener, len(targets))
	for i, target := range targets {
		ret[i], err = target.Listen()
		if err != nil {
			// Close everything up to this listener
			for x := range i {
				ret[x].Close()
			}
			return nil, err
		}
	}
	return ret, nil
}

// ListenPacketAll creates UDP connections from SERVER_STARTER_PORT.
func ListenPacketAll() ([]net.PacketConn, error) {
	targets, err := parseListenTargets(GetPortsSpecification())
	if err != nil {
		return nil, err
	}
	ret := make([]net.PacketConn, len(targets))
	for i, target := range targets {
		udp, ok := target.(UDPListener)
		if !ok {
			for x := range i {
				ret[x].Close()
			}
			return nil, fmt.Errorf("listen target %q is not UDP", target.String())
		}
		ret[i], err = udp.ListenPacket()
		if err != nil {
			for x := range i {
				ret[x].Close()
			}
			return nil, err
		}
	}
	return ret, nil
}

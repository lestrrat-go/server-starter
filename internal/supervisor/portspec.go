package supervisor

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"syscall"

	starter "github.com/lestrrat-go/server-starter/v2"
)

type listener struct {
	listener net.Listener
	packet   net.PacketConn
	fd       int

	// network, host, and port describe a TCP/UDP listener's bind target
	// (network is "tcp4"/"tcp6"/"udp4"/"udp6"); path describes a unix
	// listener's socket path, with network set to "unix". Captured at bind
	// time in Run so startWorker can format the SERVER_STARTER_PORT entry
	// for this listener through the root package's List/Listener types
	// instead of an inline format string.
	network string
	host    string
	port    int
	path    string
}

// starterListener converts l, bound to fd, into the root package's Listener
// representation. Formatting the port spec this way (String()) rather than
// with an inline fmt.Sprintf keeps the supervisor's writer and the worker's
// reader (starter.ParsePorts) built from the same constructors.
func (l listener) starterListener(fd int) starter.Listener {
	switch {
	case l.network == "unix":
		return starter.NewUnixListener(l.path, uintptr(fd))
	case strings.HasPrefix(l.network, "udp"):
		return starter.NewUDPListener(l.host, l.port, uintptr(fd))
	default:
		return starter.NewTCPListener(l.host, l.port, uintptr(fd))
	}
}

type portTarget struct {
	host    string
	port    int
	network string
	spec    string
	fd      int
}

func parsePortTarget(raw string) (portTarget, error) {
	target := strings.TrimSpace(raw)
	fd := -1
	if i := strings.LastIndexByte(target, '='); i >= 0 {
		value, err := strconv.Atoi(strings.TrimSpace(target[i+1:]))
		if err != nil || value < 0 {
			return portTarget{}, fmt.Errorf("invalid file descriptor in %q", raw)
		}
		fd = value
		target = strings.TrimSpace(target[:i])
	}

	udp := strings.HasPrefix(target, "u")
	if udp {
		target = strings.TrimPrefix(target, "u")
	}
	host := ""
	portText := target
	if strings.HasPrefix(target, "[") {
		var err error
		host, portText, err = net.SplitHostPort(target)
		if err != nil {
			return portTarget{}, fmt.Errorf("invalid address %q: %w", raw, err)
		}
	} else if i := strings.LastIndexByte(target, ':'); i >= 0 {
		host = target[:i]
		portText = target[i+1:]
	}
	if strings.HasPrefix(portText, "u") {
		udp = true
		portText = strings.TrimPrefix(portText, "u")
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 0 || port > 65535 {
		return portTarget{}, fmt.Errorf("invalid port in %q", raw)
	}
	network := "tcp4"
	if udp {
		network = "udp4"
	}
	if strings.Contains(host, ":") {
		if udp {
			network = "udp6"
		} else {
			network = "tcp6"
		}
	}
	spec := strconv.Itoa(port)
	if host != "" {
		spec = net.JoinHostPort(host, strconv.Itoa(port))
	}
	if udp {
		spec = "u" + spec
	}
	return portTarget{host: host, port: port, network: network, spec: spec, fd: fd}, nil
}

func listenConfig(network string) net.ListenConfig {
	return net.ListenConfig{Control: func(_, _ string, conn syscall.RawConn) error {
		var controlErr error
		if err := conn.Control(func(fd uintptr) {
			controlErr = setSockOptReuseAddr(fd)
			if controlErr == nil && strings.HasSuffix(network, "6") {
				controlErr = setSockOptIPv6Only(fd)
			}
		}); err != nil {
			return err
		}
		return controlErr
	}}
}

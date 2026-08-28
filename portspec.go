package starter

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"syscall"
)

type listener struct {
	listener net.Listener
	packet   net.PacketConn
	fd       int
	spec     string // path or port spec
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

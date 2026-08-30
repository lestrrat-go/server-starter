package supervisor

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"syscall"

	starter "github.com/lestrrat-go/server-starter/v2"
	"github.com/lestrrat-go/server-starter/v2/internal/portwire"
)

const (
	unixNetwork = "unix"
	udp4Network = "udp4"

	// Explicit descriptors are a convenience for integrations that require a
	// stable inherited descriptor number. Keep them bounded because ExtraFiles
	// must materialize every slot from descriptor 3 through the largest one.
	maxInheritedListenerFD = 1024

	// Sparse layouts are padded with open files. Limit the padding separately
	// so a single valid-but-distant descriptor cannot consume hundreds of
	// process descriptors before the worker starts.
	maxSparseListenerFDSlots = 256
)

type listener struct {
	listener net.Listener
	packet   net.PacketConn

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

	// socketIdentity identifies the socket entry created by Run. Teardown uses
	// it to distinguish that entry from a replacement installed at the same path.
	socketIdentity *socketIdentity
}

// starterListener converts l, bound to fd, into the root package's Listener
// representation. Formatting the port spec through starter.FormatPorts rather
// than with an inline fmt.Sprintf keeps the supervisor's writer and the
// worker's reader (starter.ParsePorts) on the same validation rules.
func (l listener) starterListener(fd int) starter.Listener {
	switch {
	case l.network == unixNetwork:
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
		if err := validateExplicitListenerFD(value); err != nil {
			return portTarget{}, fmt.Errorf("invalid file descriptor in %q: %w", raw, err)
		}
		fd = value
		target = strings.TrimSpace(target[:i])
	}
	if err := validatePortTargetDelimiter(target); err != nil {
		return portTarget{}, err
	}

	parsed, err := starter.ParsePorts(target + "=3")
	if err != nil || len(parsed) != 1 {
		return portTarget{}, fmt.Errorf("invalid port in %q", raw)
	}

	var host string
	var port int
	network := ""
	spec := strings.TrimSuffix(parsed[0].String(), "=3")
	switch target := parsed[0].(type) {
	case starter.TCPListener:
		host = target.Addr
		port = target.Port
		network = "tcp4"
	case starter.UDPListener:
		host = target.Addr
		port = target.Port
		network = udp4Network
	default:
		return portTarget{}, fmt.Errorf("invalid port in %q", raw)
	}
	if !portwire.ValidPort(int64(port)) {
		return portTarget{}, fmt.Errorf("invalid port in %q", raw)
	}
	if host == "0.0.0.0" {
		host = ""
	}
	if strings.Contains(host, ":") {
		if strings.HasPrefix(network, "udp") {
			network = "udp6"
		} else {
			network = "tcp6"
		}
	}
	return portTarget{host: host, port: port, network: network, spec: spec, fd: fd}, nil
}

// validatePortTargetDelimiter preserves the public FormatPorts error for a
// semicolon in a CLI target. ParsePorts cannot inspect that target directly
// because it correctly treats the semicolon as a SERVER_STARTER_PORT entry
// separator.
func validatePortTargetDelimiter(target string) error {
	if !strings.ContainsRune(target, ';') {
		return nil
	}

	udp := false
	if explicitTarget, ok := strings.CutPrefix(target, "udp://"); ok {
		udp = true
		target = explicitTarget
	}

	host := ""
	portText := target
	if strings.HasPrefix(target, "[") {
		var err error
		host, portText, err = net.SplitHostPort(target)
		if err != nil {
			return fmt.Errorf("invalid address %q: %w", target, err)
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
	if err != nil {
		return fmt.Errorf("invalid port in %q: %w", target, err)
	}
	if !portwire.ValidPort(int64(port)) {
		return fmt.Errorf("invalid port in %q: must be between 0 and 65535", target)
	}

	if udp {
		_, err = starter.FormatPorts(starter.NewUDPListener(host, port, 0))
	} else {
		_, err = starter.FormatPorts(starter.NewTCPListener(host, port, 0))
	}
	return err
}

// validateListenerWireFormat applies the public SERVER_STARTER_PORT encoder
// before Run binds anything. The descriptors are already assigned at this
// point, so this validates the same Listener values startWorker will later
// format for the child process.
func validateListenerWireFormat(targets []portTarget, paths []string, descriptors []int) error {
	listeners := make(starter.List, 0, len(targets)+len(paths))
	for i, target := range targets {
		l := listener{
			network: target.network,
			host:    target.host,
			port:    target.port,
		}
		listeners = append(listeners, l.starterListener(descriptors[i]))
	}
	for i, path := range paths {
		l := listener{network: "unix", path: path}
		listeners = append(listeners, l.starterListener(descriptors[len(targets)+i]))
	}

	if len(listeners) == 0 {
		return nil
	}
	_, err := starter.FormatPorts(listeners...)
	return err
}

func validateExplicitListenerFD(fd int) error {
	if fd < 0 || !portwire.ValidInheritedFD(uint64(fd)) {
		return fmt.Errorf("listener descriptor %d conflicts with standard streams", fd)
	}
	if fd > maxInheritedListenerFD {
		return fmt.Errorf("listener descriptor %d exceeds maximum %d", fd, maxInheritedListenerFD)
	}
	return nil
}

// assignListenerDescriptors validates requested descriptor numbers and fills
// automatic entries (represented by -1) from descriptor 3 upward. The
// returned slice is safe to use as indexes into exec.Cmd.ExtraFiles.
func assignListenerDescriptors(requested []int) ([]int, error) {
	descriptors := make([]int, len(requested))
	used := make(map[int]struct{}, len(requested))
	maxFD := 2
	for i, fd := range requested {
		if fd == -1 {
			continue
		}
		if err := validateExplicitListenerFD(fd); err != nil {
			return nil, err
		}
		if _, ok := used[fd]; ok {
			return nil, fmt.Errorf("listener descriptor %d is specified more than once", fd)
		}
		descriptors[i] = fd
		used[fd] = struct{}{}
		if fd > maxFD {
			maxFD = fd
		}
	}

	padding := maxFD - 2 - len(requested)
	if padding > maxSparseListenerFDSlots {
		return nil, fmt.Errorf(
			"listener descriptor layout requires %d unused slots; maximum is %d",
			padding,
			maxSparseListenerFDSlots,
		)
	}

	nextFD := 3
	for i := range descriptors {
		if descriptors[i] != 0 {
			continue
		}
		for {
			if _, ok := used[nextFD]; !ok {
				descriptors[i] = nextFD
				used[nextFD] = struct{}{}
				nextFD++
				break
			}
			nextFD++
		}
	}
	return descriptors, nil
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

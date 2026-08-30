package starter

import (
	"fmt"
	"net"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/lestrrat-go/server-starter/v2/internal/portwire"
)

// Being lazy here...
var reLooksLikeHostPort = regexp.MustCompile(`^(.+?):(\d+)$`)
var reLooksLikePort = regexp.MustCompile(`^\d+$`)

// looksLikeTCPGrammar reports whether s parses as a bare port, "host:port",
// or "[ipv6]:port" — the grammar shared by TCP and UDP specs.
func looksLikeTCPGrammar(s string) bool {
	if reLooksLikePort.MatchString(s) {
		return true
	}
	host, port, err := net.SplitHostPort(s)
	return err == nil && host != "" && reLooksLikePort.MatchString(port)
}

// stripLeadingUDPMarker accepts Server::Starter's leading "u" marker for a
// bare UDP port. A leading u in a hostname remains part of that hostname.
func stripLeadingUDPMarker(s string) (string, bool) {
	stripped, ok := strings.CutPrefix(s, "u")
	if ok && reLooksLikePort.MatchString(stripped) {
		return stripped, true
	}
	return s, false
}

// stripTrailingUDPMarker strips a "u" immediately after the last ":" in s,
// reporting whether s had one.
func stripTrailingUDPMarker(s string) (string, bool) {
	idx := strings.LastIndexByte(s, ':')
	if idx < 0 || !strings.HasPrefix(s[idx+1:], "u") {
		return s, false
	}
	return s[:idx+1] + strings.TrimPrefix(s[idx+1:], "u"), true
}

// udpCandidate is a candidate (udp, target) pair considered when
// classifying a spec's UDP marker.
type udpCandidate struct {
	udp    bool
	target string
}

// classifyUDPMarker recognizes the UDP forms accepted by Server::Starter: a
// bare uPORT or host:uPORT. It never writes those markers into
// SERVER_STARTER_PORT; SocketTypesEnvName preserves the type for v2 workers.
func classifyUDPMarker(hostPort string) []udpCandidate {
	var candidates []udpCandidate

	if trailingStripped, hasTrailing := stripTrailingUDPMarker(hostPort); hasTrailing {
		candidates = append(candidates, udpCandidate{udp: true, target: trailingStripped})
	}
	if leadingStripped, hasLeading := stripLeadingUDPMarker(hostPort); hasLeading {
		candidates = append(candidates, udpCandidate{udp: true, target: leadingStripped})
	}

	return append(candidates, udpCandidate{udp: false, target: hostPort})
}

// classifyPortTarget applies ParsePorts's complete TCP and UDP target
// classification, including the final host:port grammar fallback.
func classifyPortTarget(hostPort string) (bool, string, bool) {
	for _, candidate := range classifyUDPMarker(hostPort) {
		if looksLikeTCPGrammar(candidate.target) {
			return candidate.udp, candidate.target, true
		}
	}

	if reLooksLikeHostPort.MatchString(hostPort) {
		return false, hostPort, true
	}
	return false, hostPort, false
}

// canonicalUnixPath adds the existing "./" wire-format disambiguator when
// ParsePorts would otherwise classify path as TCP or UDP.
func canonicalUnixPath(path string) string {
	wireTarget := strings.TrimSpace(path)
	if strings.ContainsRune(wireTarget, '/') {
		return path
	}
	if _, _, matched := classifyPortTarget(wireTarget); matched {
		return "./" + path
	}
	return path
}

// ParsePorts parses the "spec=fd;spec=fd;..." value carried by
// SERVER_STARTER_PORT (see PortEnvName) into concrete Listener values.
//
// Each spec is classified in this order: the legacy UDP form "host:uPORT" is
// considered first, then a bare "uPORT", then a TCP port or host:port, and
// finally a Unix socket path. In the "host:uPORT" form, the suffix is the
// marker, so a leading "u" remains part of the hostname. A spec containing
// "/" is always a Unix socket path. TCP addresses and Unix socket paths cannot
// contain ";" or "=" because those characters delimit entries and file
// descriptors in the wire format.
//
// Raw relative unix socket paths that match a TCP or UDP spelling, such as
// "8080", "db:5432", "u8080", or "db:u5432", are read as that transport.
// Prefix them with "./" to disambiguate them. NewUnixListener adds the prefix
// automatically and stores the canonical path.
//
// TCP and UDP ports must be between 0 and 65535. Inherited file descriptors
// must be at least 3 so they do not overlap the standard streams.
//
// An empty spec returns an empty List. Ports applies the environment-specific
// requirement that SERVER_STARTER_PORT contain at least one target.
func ParsePorts(spec string) (List, error) {
	if spec == "" {
		return nil, nil
	}

	rawspec := strings.Split(spec, ";")
	ret := make(List, len(rawspec))

	for i, pairString := range rawspec {
		pair := strings.SplitN(pairString, "=", 2)
		if len(pair) != 2 {
			return nil, fmt.Errorf("failed to parse '%s' as listen target: expected exactly one '='", pairString)
		}
		rawTarget := pair[0]
		hostPort := strings.TrimSpace(rawTarget)
		fdString := strings.TrimSpace(pair[1])
		fd, err := strconv.ParseUint(fdString, 10, 0)
		if err != nil {
			return nil, fmt.Errorf("failed to parse '%s' as listen target: %s", pairString, err)
		}
		if !portwire.ValidInheritedFD(fd) {
			return nil, fmt.Errorf("failed to parse '%s' as listen target: file descriptor must be at least 3", pairString)
		}

		if strings.ContainsRune(hostPort, '/') {
			ret[i] = NewUnixListener(rawTarget, uintptr(fd))
			continue
		}

		udp, target, _ := classifyPortTarget(hostPort)

		if matches := reLooksLikeHostPort.FindStringSubmatch(target); matches != nil {
			port, err := strconv.ParseInt(matches[2], 10, 0)
			if err != nil {
				return nil, err
			}
			if !portwire.ValidPort(port) {
				return nil, fmt.Errorf("invalid port in %q", pairString)
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
			if !portwire.ValidPort(port) {
				return nil, fmt.Errorf("invalid port in %q", pairString)
			}

			if udp {
				ret[i] = UDPListener{Addr: wildcardIPv4, Port: int(port), fd: uintptr(fd)}
			} else {
				ret[i] = TCPListener{Addr: wildcardIPv4, Port: int(port), fd: uintptr(fd)}
			}
		} else {
			ret[i] = NewUnixListener(rawTarget, uintptr(fd))
		}
	}

	return ret, nil
}

// FormatPorts encodes ls into the "spec=fd;spec=fd;..." wire format carried
// by SERVER_STARTER_PORT. It matches Perl's Server::Starter format, which does
// not encode whether a network socket is TCP or UDP. Use FormatSocketTypes to
// encode that extra detail for v2 workers.
//
// ls is variadic so a single listener can be formatted ad-hoc, and so a
// List can be passed directly as FormatPorts(list...).
//
// An empty list returns an empty string. TCP and Unix listeners round-trip
// through ParsePorts. UDP listeners require SocketTypesEnvName to retain their
// transport type.
//
// FormatPorts rejects the following inputs:
//
//   - a TCPListener or UDPListener with an empty Addr
//   - a UnixListener with an empty Path
//   - a TCPListener or UDPListener whose Addr contains a NUL byte, or a
//     UnixListener whose Path contains one; environment variables cannot
//     carry NUL bytes
//   - a TCPListener or UDPListener whose Addr, or a UnixListener whose Path,
//     contains ';' (the spec separator) or '=' (the spec/fd separator),
//     either of which ParsePorts would misread
//   - any Listener whose concrete type is not TCPListener, UDPListener, or
//     UnixListener; FormatPorts has no way to validate an implementation
//     it does not know, so it refuses to guess rather than risk emitting
//     a spec ParsePorts cannot read back correctly
//   - a listener whose String result ParsePorts would read back as a
//     different listener, including an ambiguous relative unix socket path
//     built as a struct literal instead of with NewUnixListener
func FormatPorts(ls ...Listener) (string, error) {
	if len(ls) == 0 {
		return "", nil
	}

	specs := make([]string, len(ls))
	for i, l := range ls {
		switch v := l.(type) {
		case TCPListener:
			if v.Addr == "" {
				return "", fmt.Errorf("starter: cannot format TCPListener (port %d): Addr is empty", v.Port)
			}
			if strings.ContainsRune(v.Addr, '\x00') {
				return "", fmt.Errorf(
					"starter: cannot format TCPListener (port %d): Addr contains a NUL byte",
					v.Port,
				)
			}
			if strings.ContainsAny(v.Addr, ";=") {
				return "", fmt.Errorf(
					"starter: cannot format TCPListener (address %q): Addr must not contain ';' or '='",
					v.Addr,
				)
			}
		case UDPListener:
			if v.Addr == "" {
				return "", fmt.Errorf("starter: cannot format UDPListener (port %d): Addr is empty", v.Port)
			}
			if strings.ContainsRune(v.Addr, '\x00') {
				return "", fmt.Errorf(
					"starter: cannot format UDPListener (port %d): Addr contains a NUL byte",
					v.Port,
				)
			}
			if strings.ContainsAny(v.Addr, ";=") {
				return "", fmt.Errorf(
					"starter: cannot format UDPListener (address %q): Addr must not contain ';' or '='",
					v.Addr,
				)
			}
		case UnixListener:
			if v.Path == "" {
				return "", fmt.Errorf("starter: cannot format UnixListener: Path is empty")
			}
			if strings.ContainsRune(v.Path, '\x00') {
				return "", fmt.Errorf("starter: cannot format UnixListener: Path contains a NUL byte")
			}
			if strings.ContainsAny(v.Path, ";=") {
				return "", fmt.Errorf("starter: cannot format UnixListener (path %q): Path must not contain ';' or '='", v.Path)
			}
		default:
			return "", fmt.Errorf("starter: cannot format listener of type %T: unsupported Listener implementation", l)
		}
		specs[i] = l.String()
	}

	spec := strings.Join(specs, ";")
	parsed, err := ParsePorts(spec)
	if err != nil {
		return "", fmt.Errorf("starter: cannot format listeners: encoded value is not parseable: %w", err)
	}
	if len(parsed) != len(ls) {
		return "", fmt.Errorf(
			"starter: cannot format listeners: encoded value parses as %d listeners instead of %d",
			len(parsed),
			len(ls),
		)
	}
	for i, l := range ls {
		if udp, ok := l.(UDPListener); ok {
			if parsed[i] == NewTCPListener(udp.Addr, udp.Port, udp.Fd()) {
				continue
			}
		}
		if parsed[i] != l {
			return "", fmt.Errorf(
				"starter: cannot format listener %d (%T): encoded value parses as %T with different fields",
				i,
				l,
				parsed[i],
			)
		}
	}

	return spec, nil
}

// FormatSocketTypes encodes listener types as "fd=type" entries joined by
// ";" for SocketTypesEnvName. It is used together with FormatPorts: the
// latter remains compatible with Perl's SERVER_STARTER_PORT format, while this
// value lets v2 workers distinguish UDP sockets from TCP sockets.
func FormatSocketTypes(ls ...Listener) (string, error) {
	if len(ls) == 0 {
		return "", nil
	}

	types := make([]string, len(ls))
	seen := make(map[uintptr]struct{}, len(ls))
	for i, l := range ls {
		fd := l.Fd()
		if _, ok := seen[fd]; ok {
			return "", fmt.Errorf("starter: cannot format socket types: duplicate file descriptor %d", fd)
		}
		seen[fd] = struct{}{}

		var socketType string
		switch l.(type) {
		case TCPListener:
			socketType = "tcp"
		case UDPListener:
			socketType = "udp"
		case UnixListener:
			socketType = "unix"
		default:
			return "", fmt.Errorf("starter: cannot format socket type for %T", l)
		}
		types[i] = strconv.FormatUint(uint64(fd), 10) + "=" + socketType
	}
	return strings.Join(types, ";"), nil
}

func applySocketTypes(list List, spec string) (List, error) {
	types := make(map[uintptr]string, len(list))
	for _, entry := range strings.Split(spec, ";") {
		fdText, socketType, ok := strings.Cut(entry, "=")
		if !ok || fdText == "" || socketType == "" {
			return nil, fmt.Errorf("starter: invalid %s entry %q", SocketTypesEnvName, entry)
		}
		fd, err := strconv.ParseUint(fdText, 10, 0)
		if err != nil {
			return nil, fmt.Errorf("starter: invalid %s descriptor %q: %w", SocketTypesEnvName, fdText, err)
		}
		key := uintptr(fd)
		if _, ok := types[key]; ok {
			return nil, fmt.Errorf("starter: duplicate %s descriptor %d", SocketTypesEnvName, key)
		}
		types[key] = socketType
	}

	ret := make(List, len(list))
	for i, listener := range list {
		socketType, ok := types[listener.Fd()]
		if !ok {
			return nil, fmt.Errorf("starter: %s has no type for descriptor %d", SocketTypesEnvName, listener.Fd())
		}
		delete(types, listener.Fd())

		switch typed := listener.(type) {
		case TCPListener:
			switch socketType {
			case "tcp":
				ret[i] = typed
			case "udp":
				ret[i] = NewUDPListener(typed.Addr, typed.Port, typed.Fd())
			default:
				return nil, fmt.Errorf("starter: %s type %q conflicts with TCP descriptor %d", SocketTypesEnvName, socketType, typed.Fd())
			}
		case UnixListener:
			if socketType != "unix" {
				return nil, fmt.Errorf("starter: %s type %q conflicts with Unix descriptor %d", SocketTypesEnvName, socketType, typed.Fd())
			}
			ret[i] = typed
		default:
			return nil, fmt.Errorf("starter: cannot apply %s to %T", SocketTypesEnvName, listener)
		}
	}
	if len(types) != 0 {
		return nil, fmt.Errorf("starter: %s has descriptors that are not in %s", SocketTypesEnvName, PortEnvName)
	}
	return ret, nil
}

// Ports parses environment variable SERVER_STARTER_PORT (see PortEnvName).
// The returned List can contain TCPListener, UnixListener, and UDPListener
// values. UDPListener values require SocketTypesEnvName, which this
// implementation sets for its workers. For a mixed list, type-switch on
// UDPListener and call ListenPacket; call Listen on the other built-in types.
//
// It returns ErrNoListeningTarget when the variable is empty or unset.
func Ports() (List, error) {
	spec := os.Getenv(PortEnvName)
	if spec == "" {
		return nil, ErrNoListeningTarget
	}
	list, err := ParsePorts(spec)
	if err != nil {
		return nil, err
	}
	types, ok := os.LookupEnv(SocketTypesEnvName)
	if !ok || types == "" {
		return list, nil
	}
	return applySocketTypes(list, types)
}

// ListenAll parses SERVER_STARTER_PORT and creates net.Listener objects. It is
// for lists containing only TCP and unix endpoints and returns an error when a
// UDPListener is present. Use Ports and a type switch for mixed lists.
func ListenAll() ([]net.Listener, error) {
	targets, err := Ports()
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

// ListenPacketAll creates UDP connections from SERVER_STARTER_PORT. It is for
// lists containing only UDP endpoints and returns an error when a TCPListener,
// UnixListener, or custom Listener is present. Use Ports and a type switch for
// mixed lists.
func ListenPacketAll() ([]net.PacketConn, error) {
	targets, err := Ports()
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

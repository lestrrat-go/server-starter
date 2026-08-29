package starter

import (
	"fmt"
	"net"
	"net/netip"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/lestrrat-go/server-starter/v2/internal/portwire"
)

// Being lazy here...
var reLooksLikeHostPort = regexp.MustCompile(`^(.+?):(\d+)$`)
var reLooksLikePort = regexp.MustCompile(`^\d+$`)

const udpTransportMarker = "udp://"

// looksLikeTCPGrammar reports whether s parses as a bare port, "host:port",
// or "[ipv6]:port" — the grammar shared by TCP and UDP specs.
func looksLikeTCPGrammar(s string) bool {
	if reLooksLikePort.MatchString(s) {
		return true
	}
	host, port, err := net.SplitHostPort(s)
	return err == nil && host != "" && reLooksLikePort.MatchString(port)
}

func isIPLiteral(host string) bool {
	if strings.HasPrefix(host, "[") && strings.HasSuffix(host, "]") {
		host = host[1 : len(host)-1]
	}
	_, err := netip.ParseAddr(host)
	return err == nil
}

// stripLeadingUDPMarker accepts the leading "u" marker only when the
// remainder is a bare port or an IP literal with a port. Restricting the
// marker this way keeps ordinary TCP hostnames such as "upstream" from
// being consumed as UDP.
func stripLeadingUDPMarker(s string) (string, bool) {
	stripped, ok := strings.CutPrefix(s, "u")
	if !ok {
		return s, false
	}
	if reLooksLikePort.MatchString(stripped) {
		return stripped, true
	}
	matches := reLooksLikeHostPort.FindStringSubmatch(stripped)
	if matches == nil || !isIPLiteral(matches[1]) {
		return s, false
	}
	return stripped, true
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

// classifyUDPMarker returns, in priority order, the candidate transport
// interpretations of hostPort. The explicit "udp://" marker wins. A trailing
// legacy marker is authoritative, then a constrained leading marker is tried
// for bare ports and IP literals. An unmarked TCP target is tried last.
func classifyUDPMarker(hostPort string) []udpCandidate {
	if target, ok := strings.CutPrefix(hostPort, udpTransportMarker); ok {
		return []udpCandidate{{udp: true, target: target}}
	}

	var candidates []udpCandidate

	if trailingStripped, hasTrailing := stripTrailingUDPMarker(hostPort); hasTrailing {
		candidates = append(candidates, udpCandidate{udp: true, target: trailingStripped})
	}
	if leadingStripped, hasLeading := stripLeadingUDPMarker(hostPort); hasLeading {
		candidates = append(candidates, udpCandidate{udp: true, target: leadingStripped})
	}

	return append(candidates, udpCandidate{udp: false, target: hostPort})
}

// ParsePorts parses the "spec=fd;spec=fd;..." value carried by
// SERVER_STARTER_PORT (see PortEnvName) into concrete Listener values.
//
// Each spec is classified in this order: a spec beginning with "udp://" is
// a UDP target; otherwise the legacy UDP form "host:uPORT" is considered;
// otherwise the constrained leading-marker forms "uPORT", "uIPv4:PORT", and
// "u[IPv6]:PORT" are considered; otherwise a spec that parses as a
// port/host:port is a TCP target; otherwise the spec is a unix socket path,
// taken verbatim. In the "host:uPORT" form, the suffix is the marker, so a
// leading "u" remains part of the hostname. A spec containing "/" is always a
// unix socket path unless it begins with "udp://". TCP/UDP addresses and unix
// socket paths cannot contain ";" or "=" because those characters delimit
// entries and file descriptors in the wire format.
//
// This leaves one shape ambiguous: a relative unix socket path with no "/"
// that happens to parse as a port or "host:port" (e.g. "8080" or "db:5432")
// is read as TCP, not as a unix socket. Pass such sockets as absolute
// paths, or prefix them with "./" to disambiguate.
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
		hostPort := strings.TrimSpace(pair[0])
		fdString := strings.TrimSpace(pair[1])
		fd, err := strconv.ParseUint(fdString, 10, 0)
		if err != nil {
			return nil, fmt.Errorf("failed to parse '%s' as listen target: %s", pairString, err)
		}
		if !portwire.ValidInheritedFD(fd) {
			return nil, fmt.Errorf("failed to parse '%s' as listen target: file descriptor must be at least 3", pairString)
		}

		explicitUDP := strings.HasPrefix(hostPort, udpTransportMarker)
		if !explicitUDP && strings.ContainsRune(hostPort, '/') {
			ret[i] = UnixListener{Path: hostPort, fd: uintptr(fd)}
			continue
		}

		udp := false
		target := hostPort
		matched := false
		for _, c := range classifyUDPMarker(hostPort) {
			if looksLikeTCPGrammar(c.target) {
				udp = c.udp
				target = c.target
				matched = true
				break
			}
		}
		if explicitUDP && !matched {
			return nil, fmt.Errorf("failed to parse %q as UDP listen target", pairString)
		}

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
			ret[i] = UnixListener{
				Path: hostPort,
				fd:   uintptr(fd),
			}
		}
	}

	return ret, nil
}

// FormatPorts encodes ls into the "spec=fd;spec=fd;..." wire format read
// by ParsePorts and carried by SERVER_STARTER_PORT. It is the authoritative
// encoder for that format: each Listener's own String() method is a
// display form for humans and does not validate its receiver, so a
// TCPListener, UDPListener, or UnixListener built directly as a struct
// literal (bypassing New*Listener's normalisation) can render into a spec
// that ParsePorts reads back as something else entirely, with no error
// anywhere in the chain. FormatPorts closes that gap by rejecting a
// malformed listener instead of silently encoding it.
//
// ls is variadic so a single listener can be formatted ad-hoc, and so a
// List can be passed directly as FormatPorts(list...).
//
// An empty list returns an empty string so FormatPorts and ParsePorts are
// inverses for that valid parse-layer value.
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
//     different listener, including ambiguous relative unix socket paths
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

// Ports parses environment variable SERVER_STARTER_PORT (see PortEnvName).
// The returned List can contain TCPListener, UnixListener, and UDPListener
// values. For a mixed list, type-switch on UDPListener and call ListenPacket;
// call Listen on the other built-in Listener values.
//
// It returns ErrNoListeningTarget when the variable is empty or unset.
func Ports() (List, error) {
	spec := os.Getenv(PortEnvName)
	if spec == "" {
		return nil, ErrNoListeningTarget
	}
	return ParsePorts(spec)
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

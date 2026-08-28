package starter

import (
	"fmt"
	"net"
	"os"
	"regexp"
	"strconv"
	"strings"
)

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

// stripLeadingUDPMarker strips a leading "u" from s, reporting whether s
// had one.
func stripLeadingUDPMarker(s string) (string, bool) {
	stripped := strings.TrimPrefix(s, "u")
	return stripped, stripped != s
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

// classifyUDPMarker returns, in priority order, the candidate
// interpretations of hostPort's UDP marker(s): both a leading "u" and a
// trailing ":u" stripped, only the leading "u" stripped, only the trailing
// ":u" stripped, and finally no strip at all. The caller picks the first
// candidate whose target satisfies looksLikeTCPGrammar.
func classifyUDPMarker(hostPort string) []udpCandidate {
	var candidates []udpCandidate

	if leadingStripped, hasLeading := stripLeadingUDPMarker(hostPort); hasLeading {
		if bothStripped, hasTrailing := stripTrailingUDPMarker(leadingStripped); hasTrailing {
			candidates = append(candidates, udpCandidate{udp: true, target: bothStripped})
		}
		candidates = append(candidates, udpCandidate{udp: true, target: leadingStripped})
	}
	if trailingStripped, hasTrailing := stripTrailingUDPMarker(hostPort); hasTrailing {
		candidates = append(candidates, udpCandidate{udp: true, target: trailingStripped})
	}

	return append(candidates, udpCandidate{udp: false, target: hostPort})
}

// ParsePorts parses the "spec=fd;spec=fd;..." value carried by
// SERVER_STARTER_PORT (see PortEnvName) into concrete Listener values.
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
func ParsePorts(spec string) (List, error) {
	if spec == "" {
		return nil, ErrNoListeningTarget
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

		if strings.ContainsRune(hostPort, '/') {
			ret[i] = UnixListener{Path: hostPort, fd: uintptr(fd)}
			continue
		}

		udp := false
		target := hostPort
		for _, c := range classifyUDPMarker(hostPort) {
			if looksLikeTCPGrammar(c.target) {
				udp = c.udp
				target = c.target
				break
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
// FormatPorts rejects, by returning an error naming the offending listener:
//
//   - a TCPListener or UDPListener with an empty Addr
//   - a UnixListener with an empty Path
//   - a UnixListener whose Path contains ';' (the spec separator) or '='
//     (the spec/fd separator), either of which ParsePorts would misread
//   - any Listener whose concrete type is not TCPListener, UDPListener, or
//     UnixListener; FormatPorts has no way to validate an implementation
//     it does not know, so it refuses to guess rather than risk emitting
//     a spec ParsePorts cannot read back correctly
func FormatPorts(ls ...Listener) (string, error) {
	specs := make([]string, len(ls))
	for i, l := range ls {
		switch v := l.(type) {
		case TCPListener:
			if v.Addr == "" {
				return "", fmt.Errorf("starter: cannot format TCPListener (port %d): Addr is empty", v.Port)
			}
		case UDPListener:
			if v.Addr == "" {
				return "", fmt.Errorf("starter: cannot format UDPListener (port %d): Addr is empty", v.Port)
			}
		case UnixListener:
			if v.Path == "" {
				return "", fmt.Errorf("starter: cannot format UnixListener: Path is empty")
			}
			if strings.ContainsAny(v.Path, ";=") {
				return "", fmt.Errorf("starter: cannot format UnixListener (path %q): Path must not contain ';' or '='", v.Path)
			}
		default:
			return "", fmt.Errorf("starter: cannot format listener of type %T: unsupported Listener implementation", l)
		}
		specs[i] = l.String()
	}
	return strings.Join(specs, ";"), nil
}

// Ports parses environment variable SERVER_STARTER_PORT (see PortEnvName).
func Ports() (List, error) {
	return ParsePorts(os.Getenv(PortEnvName))
}

// ListenAll parses environment variable SERVER_STARTER_PORT, and creates
// net.Listener objects
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

// ListenPacketAll creates UDP connections from SERVER_STARTER_PORT.
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

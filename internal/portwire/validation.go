// Package portwire defines validation shared by the supervisor and workers
// for values carried in SERVER_STARTER_PORT.
package portwire

const (
	minPort        = 0
	maxPort        = 65535
	minInheritedFD = 3
)

// ValidPort reports whether port can identify a TCP or UDP port.
func ValidPort(port int64) bool {
	return port >= minPort && port <= maxPort
}

// ValidInheritedFD reports whether fd does not overlap the standard streams.
func ValidInheritedFD(fd uint64) bool {
	return fd >= minInheritedFD
}

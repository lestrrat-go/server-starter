//go:build !windows

package supervisor

import "syscall"

// setSockOptReuseAddr sets SO_REUSEADDR on the listening socket so a port
// left in TIME_WAIT can be rebound immediately, matching the traditional
// server_starter behavior on unix.
func setSockOptReuseAddr(fd uintptr) error {
	return syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1)
}

// setSockOptIPv6Only sets IPV6_V6ONLY on the listening socket so an IPv6
// listener does not also accept IPv4-mapped connections.
func setSockOptIPv6Only(fd uintptr) error {
	return syscall.SetsockoptInt(int(fd), syscall.IPPROTO_IPV6, syscall.IPV6_V6ONLY, 1)
}

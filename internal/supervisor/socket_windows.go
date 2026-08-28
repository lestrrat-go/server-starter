package supervisor

import "golang.org/x/sys/windows"

// setSockOptReuseAddr is a deliberate no-op on Windows. SO_REUSEADDR does
// not mean the same thing there as it does on unix: on unix it mainly lets
// a listener rebind a port still in TIME_WAIT, but on Windows it lets a
// second socket bind a port another socket is actively listening on,
// letting an unrelated process hijack the listener. There is no Windows
// equivalent of the unix behavior this option emulates, so a literal
// translation would trade a convenience for a security hazard. Skip it and
// keep the Windows listener locked to the port it holds.
func setSockOptReuseAddr(fd uintptr) error {
	return nil
}

// setSockOptIPv6Only sets IPV6_V6ONLY on the listening socket so an IPv6
// listener does not also accept IPv4-mapped connections.
func setSockOptIPv6Only(fd uintptr) error {
	return windows.SetsockoptInt(windows.Handle(fd), windows.IPPROTO_IPV6, windows.IPV6_V6ONLY, 1)
}

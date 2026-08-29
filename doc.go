// Package starter provides the worker side of the start_server protocol: it
// picks up listening sockets that the start_server supervisor bound and
// passed down as inherited file descriptors.
//
// A program that wants to run under start_server imports this package and
// recovers the sockets the supervisor already bound. ListenAll handles lists
// containing only TCP and unix sockets, while ListenPacketAll handles lists
// containing only UDP sockets. Programs with a mixed list call Ports and use a
// type switch: UDPListener values use ListenPacket, and the other built-in
// Listener values use Listen.
//
// The supervisor itself is not importable through this module: it ships as
// the start_server command. See cmd/start_server.
package starter

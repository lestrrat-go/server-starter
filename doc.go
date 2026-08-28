// Package starter provides the worker side of the start_server protocol: it
// picks up listening sockets that the start_server supervisor bound and
// passed down as inherited file descriptors.
//
// A program that wants to run under start_server imports this package,
// calls ListenAll (or Ports, for finer control) to recover the sockets the
// supervisor already bound, and serves on them.
//
// The supervisor itself is not importable through this module: it ships as
// the start_server command. See cmd/start_server.
package starter

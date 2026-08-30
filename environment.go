package starter

import (
	"os"
	"strconv"
)

// PortEnvName is the environment variable that carries the listener
// specification, as a list of "spec=fd" pairs joined by ";".
//
// Each spec is a TCP target (a bare port, "host:port", or "[ipv6]:port") or a
// Unix socket path. UDP sockets use the same representation as TCP, matching
// Perl's Server::Starter. SocketTypesEnvName identifies them for v2 workers.
// Because ";" separates entries and "=" separates each target from its
// descriptor, TCP addresses and Unix socket paths cannot contain either
// delimiter. A relative Unix socket path that matches a TCP or legacy UDP
// spelling, such as "8080", "db:5432", "u8080", or "db:u5432", would
// otherwise be interpreted as a network target. Prefix raw paths with "./"
// to disambiguate them. NewUnixListener adds the prefix automatically and
// stores the canonical path.
const PortEnvName = "SERVER_STARTER_PORT"

// SocketTypesEnvName identifies socket types for workers started by this
// implementation. It contains "fd=type" entries joined by ";", where type
// is tcp, udp, or unix. Perl's Server::Starter does not set or read this
// variable, so its SERVER_STARTER_PORT format remains unchanged.
const SocketTypesEnvName = "LSS2_SOCKET_TYPES"

// GenerationEnvName is the environment variable the supervisor sets to the
// worker's generation number on every spawn. The first worker is generation
// 1, and each subsequent spawn increments it by one. The v2 supervisor does
// not set it on its own process. Unlike PortEnvName it is never empty, which
// makes it the reliable signal for IsUnderStartServer.
const GenerationEnvName = "SERVER_STARTER_GENERATION"

// IsUnderStartServer reports whether the calling process was spawned by the
// start_server supervisor.
//
// It tests GenerationEnvName, not PortEnvName. A supervisor started with no
// --port sets SERVER_STARTER_PORT to the empty string, and on Windows,
// SetEnvironmentVariableW erases a variable set to an empty value, so
// whether the port variable is present is not portable. GenerationEnvName
// is set on every worker spawn and is never empty.
func IsUnderStartServer() bool {
	_, ok := os.LookupEnv(GenerationEnvName)
	return ok
}

// Generation returns the worker's generation number, as reported by
// GenerationEnvName, and whether it was present and valid. It accepts an
// explicitly supplied generation 0 for compatibility, even though v2 workers
// start at generation 1. The bool return distinguishes an accepted zero from
// an absent or invalid value.
func Generation() (int, bool) {
	v, ok := os.LookupEnv(GenerationEnvName)
	if !ok {
		return 0, false
	}
	generation, err := strconv.Atoi(v)
	if err != nil {
		return 0, false
	}
	return generation, true
}

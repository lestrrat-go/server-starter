package starter

import (
	"os"
	"strconv"
)

// PortEnvName is the environment variable that carries the listener
// specification, as a list of "spec=fd" pairs joined by ";".
//
// Each spec is either a TCP target (a bare port, "host:port", or
// "[ipv6]:port"), a UDP target prefixed with "udp://", or a unix socket
// path. Because ";" separates entries and "=" separates each target from
// its descriptor, TCP/UDP addresses and unix socket paths cannot contain
// either delimiter. A path containing "/" is read as a unix socket unless
// it uses the UDP prefix. A relative unix socket path that matches a TCP or
// UDP spelling, such as "8080", "db:5432", "u8080", or "udp://8080", would
// otherwise be interpreted as that transport. Prefix raw paths with "./" to
// disambiguate them. NewUnixListener adds the prefix automatically and stores
// the canonical path.
const PortEnvName = "SERVER_STARTER_PORT"

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

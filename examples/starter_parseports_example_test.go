package examples_test

import (
	"fmt"

	starter "github.com/lestrrat-go/server-starter/v2"
)

// Example_starter_parseports shows the wire format carried by
// SERVER_STARTER_PORT without needing an environment variable or a real
// supervisor: a literal spec is fed straight to ParsePorts. The spec below
// covers the three listener kinds a worker can receive — a bare TCP port, a
// UDP target on a specific host:port, and a unix socket path.
func Example_starter_parseports() {
	spec := "8080=3;udp://127.0.0.1:8081=4;/tmp/app.sock=5"

	list, err := starter.ParsePorts(spec)
	if err != nil {
		fmt.Printf("failed to parse ports: %s\n", err)
		return
	}

	// List is a slice, so iterating it in order is stable and reproduces
	// the same order the spec was written in.
	for _, l := range list {
		fmt.Printf("%s (fd=%d)\n", l.String(), l.Fd())
	}

	// Output:
	// 8080=3 (fd=3)
	// udp://127.0.0.1:8081=4 (fd=4)
	// /tmp/app.sock=5 (fd=5)
}

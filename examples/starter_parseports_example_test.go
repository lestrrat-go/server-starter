package examples_test

import (
	"fmt"

	starter "github.com/lestrrat-go/server-starter/v2"
)

// Example_starter_parseports shows the wire format carried by
// SERVER_STARTER_PORT without needing an environment variable or a real
// supervisor: a literal spec is fed straight to ParsePorts. The spec below
// covers the three listener kinds a worker can receive — a TCP hostname that
// begins with "u", a UDP target on a specific host:port, and a relative unix
// socket path whose "./" prefix keeps its numeric name distinct from a TCP
// port. See
// Example_starter_mixedListeners for materializing a mixed list.
func Example_starter_parseports() {
	spec := "upstream:8080=3;udp://127.0.0.1:8081=4;./9000=5"

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
	// upstream:8080=3 (fd=3)
	// udp://127.0.0.1:8081=4 (fd=4)
	// ./9000=5 (fd=5)
}

// Example_starter_newUnixListener shows how the constructor makes a relative
// Unix path safe for a String and ParsePorts round trip. The "./" prefix names
// the same socket in the current directory while disambiguating it from a
// transport target in the wire format.
func Example_starter_newUnixListener() {
	listener := starter.NewUnixListener("8082", 5)

	fmt.Printf("stored path: %s\n", listener.Path)
	fmt.Printf("wire value: %s\n", listener.String())

	// Output:
	// stored path: ./8082
	// wire value: ./8082=5
}

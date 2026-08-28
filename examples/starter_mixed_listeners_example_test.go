package examples_test

import (
	"fmt"

	starter "github.com/lestrrat-go/server-starter/v2"
)

// Example_starter_mixedListeners shows how a worker selects the correct
// materialization method for each endpoint in a mixed list. The example uses
// ParsePorts so it can verify the routing without requiring inherited file
// descriptors from a running supervisor.
func Example_starter_mixedListeners() {
	targets, err := starter.ParsePorts("8080=3;u8081=4;/tmp/app.sock=5")
	if err != nil {
		fmt.Printf("failed to parse ports: %s\n", err)
		return
	}

	for _, target := range targets {
		switch target := target.(type) {
		case starter.UDPListener:
			// A worker calls target.ListenPacket() here and serves packets on
			// the returned net.PacketConn.
			fmt.Printf("packet: %s via ListenPacket\n", target.String())
		default:
			// TCPListener and UnixListener use target.Listen() and return a
			// net.Listener.
			fmt.Printf("stream: %s via Listen\n", target.String())
		}
	}

	// Output:
	// stream: 8080=3 via Listen
	// packet: u8081=4 via ListenPacket
	// stream: /tmp/app.sock=5 via Listen
}

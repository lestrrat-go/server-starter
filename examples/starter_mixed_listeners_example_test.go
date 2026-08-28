package examples_test

import (
	"fmt"
	"os"

	starter "github.com/lestrrat-go/server-starter/v2"
)

// Example_starter_mixedListeners shows how a worker selects the correct
// materialization method for each endpoint in a mixed list. The example sets
// SERVER_STARTER_PORT directly so it can verify the routing without inherited
// file descriptors from a running supervisor.
func Example_starter_mixedListeners() {
	prior, hadPrior := os.LookupEnv(starter.PortEnvName)
	defer func() {
		if hadPrior {
			_ = os.Setenv(starter.PortEnvName, prior)
			return
		}
		_ = os.Unsetenv(starter.PortEnvName)
	}()

	if err := os.Setenv(starter.PortEnvName, "8080=3;udp://8081=4;/tmp/app.sock=5"); err != nil {
		fmt.Printf("failed to set ports env: %s\n", err)
		return
	}

	targets, err := starter.Ports()
	if err != nil {
		fmt.Printf("failed to parse ports: %s\n", err)
		return
	}

	for _, target := range targets {
		switch target := target.(type) {
		case starter.TCPListener, starter.UnixListener:
			// TCPListener and UnixListener use target.Listen() and return a
			// net.Listener.
			fmt.Printf("stream: %s via Listen\n", target.String())
		case starter.UDPListener:
			// A worker calls target.ListenPacket() here and serves packets on
			// the returned net.PacketConn.
			fmt.Printf("packet: %s via ListenPacket\n", target.String())
		}
	}

	// Output:
	// stream: 8080=3 via Listen
	// packet: udp://8081=4 via ListenPacket
	// stream: /tmp/app.sock=5 via Listen
}

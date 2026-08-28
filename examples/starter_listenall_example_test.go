package examples_test

import (
	"fmt"
	"os"

	starter "github.com/lestrrat-go/server-starter/v2"
)

// Example_starter_listenall shows the shape a real worker uses: ask
// IsUnderStartServer first, and branch on the answer. Asking that question
// up front is what separates "not running under a supervisor" from
// "running under a supervisor but misconfigured" — calling Ports() (or
// ListenAll()) directly and pattern-matching on ErrNoListeningTarget cannot
// tell those two cases apart, because an unconfigured supervisor also
// produces that error.
func Example_starter_listenall() {
	// Guarantee this example's own environment is clean regardless of what
	// the process was started with, so its output stays fixed.
	if err := os.Unsetenv(starter.GenerationEnvName); err != nil {
		fmt.Printf("failed to unset generation env: %s\n", err)
		return
	}

	if starter.IsUnderStartServer() {
		// Under a supervisor: fetch the inherited listeners and serve on
		// them directly, e.g.:
		//
		//   listeners, err := starter.ListenAll()
		//   if err != nil {
		//       fmt.Printf("failed to listen: %s\n", err)
		//       return
		//   }
		//   for _, l := range listeners {
		//       go http.Serve(l, handler)
		//   }
		fmt.Println("running under start_server: serving on inherited listeners")
		return
	}

	// Not under a supervisor: fall back to binding a port of the worker's
	// own choosing, e.g. net.Listen("tcp", ":8080").
	fmt.Println("not running under start_server: binding own listener")

	// Output:
	// not running under start_server: binding own listener
}

package examples_test

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"sync"

	starter "github.com/lestrrat-go/server-starter/v2"
)

// Example_starter_listenall shows the shape a real worker uses: ask
// IsUnderStartServer first, and branch on the answer. Asking that question
// up front is what separates "not running under a supervisor" from
// "running under a supervisor but misconfigured" — calling Ports() (or
// ListenAll()) directly and pattern-matching on ErrNoListeningTarget cannot
// tell those two cases apart, because an unconfigured supervisor also
// produces that error. ListenAll handles only TCP and unix targets; see
// Example_starter_mixedListeners when the inherited list also contains UDP.
func Example_starter_listenall() {
	// Guarantee this example's own environment is clean regardless of what
	// the process was started with, so its output stays fixed.
	if err := os.Unsetenv(starter.GenerationEnvName); err != nil {
		fmt.Printf("failed to unset generation env: %s\n", err)
		return
	}

	if starter.IsUnderStartServer() {
		// Under a supervisor: fetch the inherited listeners and serve on
		// every one. Example_starter_serveAllListeners shows the serving
		// loop in executable form.
		//
		//   listeners, err := starter.ListenAll()
		//   if err != nil {
		//       fmt.Printf("failed to listen: %s\n", err)
		//       return
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

// Example_starter_serveAllListeners shows how a worker serves every listener
// returned by starter.ListenAll. Waiting for every serving goroutine keeps the
// worker alive while any inherited listener is still accepting requests.
func Example_starter_serveAllListeners() {
	listeners := []net.Listener{
		failedListener{name: "public", err: errors.New("public listener stopped")},
		failedListener{name: "admin", err: errors.New("admin listener stopped")},
	}
	handler := http.NotFoundHandler()

	var wg sync.WaitGroup
	for _, listener := range listeners {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := http.Serve(listener, handler); err != nil {
				fmt.Printf("failed to serve %s: %s\n", listener.Addr(), err)
			}
		}()
	}
	wg.Wait()

	// Unordered output:
	// failed to serve public: public listener stopped
	// failed to serve admin: admin listener stopped
}

type failedListener struct {
	name string
	err  error
}

func (l failedListener) Accept() (net.Conn, error) {
	return nil, l.err
}

func (failedListener) Close() error {
	return nil
}

func (l failedListener) Addr() net.Addr {
	return failedAddr(l.name)
}

type failedAddr string

func (failedAddr) Network() string {
	return "example"
}

func (a failedAddr) String() string {
	return string(a)
}

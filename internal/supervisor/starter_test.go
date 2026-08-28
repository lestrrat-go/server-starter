package supervisor

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"
)

var echoServerTxt = `package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
	starter "github.com/lestrrat-go/server-starter/v2"
)

func main() {
	// The first arg, when present, is a file this worker writes its own
	// SERVER_STARTER_PORT into. Tests use this to observe the child's view
	// of the environment without relying on the supervisor process's own
	// environment, which the supervisor must never mutate.
	if len(os.Args) > 1 {
		if err := os.WriteFile(os.Args[1], []byte(os.Getenv("SERVER_STARTER_PORT")), 0600); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to write port file: %s\n", err)
			os.Exit(1)
		}
	}

	listeners, err := starter.ListenAll()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to listen: %s\n", err)
		os.Exit(1)
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(w, r.Body)
	})
	var wg sync.WaitGroup
	for _, listener := range listeners {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := http.Serve(listener, handler); err != nil {
				fmt.Fprintf(os.Stderr, "Failed to serve %s: %s\n", listener.Addr(), err)
			}
		}()
	}
	wg.Wait()

	loop := false
	sigCh := make(chan os.Signal)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGHUP)
	for loop {
		select {
		case <-sigCh:
			loop = false
		default:
			time.Sleep(time.Second)
		}
	}
}
`

type config struct {
	args       []string
	command    string
	dir        string
	interval   int
	pidfile    string
	ports      []string
	paths      []string
	sigonhup   string
	sigonterm  string
	statusfile string

	envdir              string
	enableAutoRestart   bool
	autoRestartInterval time.Duration
	killOldDelay        time.Duration

	stdout io.Writer
	stderr io.Writer
}

func (c config) Args() []string          { return c.args }
func (c config) Command() string         { return c.command }
func (c config) Dir() string             { return c.dir }
func (c config) Interval() time.Duration { return time.Duration(c.interval) * time.Second }
func (c config) PidFile() string         { return c.pidfile }
func (c config) Ports() []string         { return c.ports }
func (c config) Paths() []string         { return c.paths }
func (c config) SignalOnHUP() os.Signal  { return SigFromName(c.sigonhup) }
func (c config) SignalOnTERM() os.Signal { return SigFromName(c.sigonterm) }
func (c config) StatusFile() string      { return c.statusfile }

func (c config) Envdir() string                     { return c.envdir }
func (c config) EnableAutoRestart() bool            { return c.enableAutoRestart }
func (c config) AutoRestartInterval() time.Duration { return c.autoRestartInterval }
func (c config) KillOldDelay() time.Duration        { return c.killOldDelay }
func (c config) Stdout() io.Writer                  { return c.stdout }
func (c config) Stderr() io.Writer                  { return c.stderr }

func TestRun(t *testing.T) {
	dir := t.TempDir()

	srcFile := filepath.Join(dir, "echod.go")
	f, err := os.OpenFile(srcFile, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0666)
	if err != nil {
		t.Errorf("Failed to create %s: %s", srcFile, err)
		return
	}
	io.WriteString(f, echoServerTxt)
	f.Close()

	// The scratch module resolves the listener package through a replace
	// directive back to this repository. The path must be absolute: a relative
	// one would depend on where t.TempDir() happens to sit under $TMPDIR.
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Errorf("Failed to resolve repository root: %s", err)
		return
	}
	goMod := fmt.Sprintf(`module server-starter-echod

go 1.23

require github.com/lestrrat-go/server-starter/v2 v2.0.0

replace github.com/lestrrat-go/server-starter/v2 => %s
`, filepath.ToSlash(root))
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(goMod), 0600); err != nil {
		t.Errorf("Failed to write go.mod: %s", err)
		return
	}

	// -buildvcs=false: the scratch module is not a checkout, and VCS stamping
	// fails outright when the build walks up into an unrelated repository.
	// -mod=mod: the scratch go.mod has no go.sum and only a bare require for
	// this module, so in the default readonly mode the build refuses to run
	// ("go: updates to go.mod needed") once this module's own dependency
	// graph gains entries (e.g. golang.org/x/sys) that the scratch module's
	// go.mod/go.sum haven't recorded yet. -mod=mod lets it resolve those on
	// the fly instead of requiring a manual "go mod tidy" here.
	cmd := exec.CommandContext(context.Background(), "go", "build", "-mod=mod", "-buildvcs=false", "-o", filepath.Join(dir, "echod"), ".")
	cmd.Dir = dir
	// GOWORK=off keeps a go.work anywhere above $TMPDIR from pulling the
	// scratch module into an unrelated workspace.
	cmd.Env = append(os.Environ(), "GOWORK=off")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Errorf("Failed to compile %s: %s\n%s", dir, err, output)
		return
	}

	reservations := make([]net.Listener, 0, 2)
	defer func() {
		for _, listener := range reservations {
			_ = listener.Close()
		}
	}()

	ports := make([]string, 0, cap(reservations))
	for range cap(reservations) {
		listener, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp4", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("failed to reserve loopback port: %s", err)
		}
		reservations = append(reservations, listener)
		ports = append(ports, listener.Addr().String())
	}
	for _, listener := range reservations {
		if err := listener.Close(); err != nil {
			t.Fatalf("failed to release loopback port: %s", err)
		}
	}
	reservations = nil

	portFile := filepath.Join(t.TempDir(), "worker-port.txt")
	sd, err := NewStarter(&config{
		ports:   ports,
		command: filepath.Join(dir, "echod"),
		args:    []string{portFile},
	})
	if err != nil {
		t.Errorf("Failed to create starter: %s", err)
		return
	}

	// The supervisor must never mutate its own process environment (two
	// concurrent supervisors would otherwise race on SERVER_STARTER_PORT).
	// Snapshotting here and comparing after the run proves that: it is a
	// stronger check than merely asserting the two variables are absent,
	// since it also catches anything unexpected being added or changed.
	before := os.Environ()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Run's setup (binding every listener) is synchronous, so the ports are
	// already live by the time Run returns; no readiness handshake needed.
	ctrl, err := sd.Run(ctx)
	if err != nil {
		t.Fatalf("sd.Run() failed: %s", err)
	}

	for _, port := range ports {
		conn, err := (&net.Dialer{}).DialContext(context.Background(), "tcp", port)
		if err != nil {
			t.Errorf("error connecting to port %q: %s", port, err)
			continue
		}
		_ = conn.Close()
	}

	time.AfterFunc(time.Second, cancel)
	if err := ctrl.Wait(); err != nil && !errors.Is(err, ErrServerClosed) {
		t.Errorf("ctrl.Wait() failed: %s", err)
	}
	t.Logf("Exiting...")

	if got := os.Environ(); !slices.Equal(got, before) {
		t.Errorf("supervisor's own environment changed during Run(): before=%v after=%v", before, got)
	}

	log.Printf("Checking ports...")

	patterns := make([]string, len(ports))
	for i, port := range ports {
		patterns[i] = fmt.Sprintf(`%s=\d+`, regexp.QuoteMeta(port))
	}
	pattern := regexp.MustCompile(strings.Join(patterns, ";"))

	// The child's view, not the supervisor's: the worker wrote its own
	// SERVER_STARTER_PORT to portFile at startup.
	childPort, err := os.ReadFile(portFile)
	if err != nil {
		t.Fatalf("failed to read worker port file: %s", err)
	}
	if !pattern.Match(childPort) {
		t.Errorf("child SERVER_STARTER_PORT: expected '%s', but got '%s'", pattern, childPort)
	}
}

// TestRunDoesNotMutateSupervisorEnvironment proves a full Run/Wait cycle
// -- including one that loads an envdir -- leaves the supervisor's own
// process environment untouched: no SERVER_STARTER_PORT, no
// SERVER_STARTER_GENERATION, and no envdir key leaks in. Those variables
// only ever land on the spawned worker's cmd.Env (see startWorker); the
// supervisor process itself must never carry them.
func TestRunDoesNotMutateSupervisorEnvironment(t *testing.T) {
	envdir := t.TempDir()
	leakKey := "SERVER_STARTER_TEST_LEAK"
	if err := os.WriteFile(filepath.Join(envdir, leakKey), []byte("leaked\n"), 0600); err != nil {
		t.Fatalf("failed to write envdir entry: %s", err)
	}

	sd, err := NewStarter(&config{
		command: "/bin/sh",
		args:    []string{"-c", "exec sleep 30"},
		ports:   []string{"0"},
		envdir:  envdir,
	})
	if err != nil {
		t.Fatalf("failed to create starter: %s", err)
	}

	before := os.Environ()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ctrl, err := sd.Run(ctx)
	if err != nil {
		t.Fatalf("sd.Run() failed: %s", err)
	}

	cancel()
	select {
	case <-ctrl.Done():
	case <-time.After(20 * time.Second):
		t.Fatal("timed out waiting for Run() to return")
	}
	if err := ctrl.Err(); err != nil && !errors.Is(err, ErrServerClosed) {
		t.Errorf("sd.Run() failed: %s", err)
	}

	if got := os.Environ(); !slices.Equal(got, before) {
		t.Errorf("supervisor's own environment changed during Run(): before=%v after=%v", before, got)
	}
	for _, key := range []string{"SERVER_STARTER_PORT", "SERVER_STARTER_GENERATION", leakKey} {
		if _, ok := os.LookupEnv(key); ok {
			t.Errorf("%s leaked into the supervisor's own environment", key)
		}
	}
}

func TestSigFromName(t *testing.T) {
	for sig, name := range niceSigNames {
		if got := SigFromName(name); sig != got {
			t.Errorf("%v: wants '%v' but got '%v'", name, sig, got)
		}
	}

	variants := map[string]syscall.Signal{
		"SIGTERM": syscall.SIGTERM,
		"sigterm": syscall.SIGTERM,
		"Hup":     syscall.SIGHUP,
	}
	for name, sig := range variants {
		if got := SigFromName(name); sig != got {
			t.Errorf("%v: wants '%v' but got '%v'", name, sig, got)
		}
	}
}

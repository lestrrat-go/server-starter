package supervisor

import (
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
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
	"syscall"
	"time"
	starter "github.com/lestrrat-go/server-starter/v2"
)

func main() {
	listeners, err := starter.ListenAll()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to listen: %s\n", err)
		os.Exit(1)
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(w, r.Body)
	})
	for _, l := range listeners {
		http.Serve(l, handler)
	}

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
	cmd := exec.Command("go", "build", "-mod=mod", "-buildvcs=false", "-o", filepath.Join(dir, "echod"), ".")
	cmd.Dir = dir
	// GOWORK=off keeps a go.work anywhere above $TMPDIR from pulling the
	// scratch module into an unrelated workspace.
	cmd.Env = append(os.Environ(), "GOWORK=off")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Errorf("Failed to compile %s: %s\n%s", dir, err, output)
		return
	}

	ports := []string{"9090", "8080"}
	sd, err := NewStarter(&config{
		ports:   ports,
		command: filepath.Join(dir, "echod"),
	})
	if err != nil {
		t.Errorf("Failed to create starter: %s", err)
		return
	}

	doneCh := make(chan struct{})
	readyCh := make(chan struct{})
	go func() {
		defer func() { doneCh <- struct{}{} }()
		time.AfterFunc(500*time.Millisecond, func() {
			readyCh <- struct{}{}
		})
		if err := sd.Run(); err != nil {
			t.Errorf("sd.Run() failed: %s", err)
		}
		t.Logf("Exiting...")
	}()

	<-readyCh

	for _, port := range ports {
		_, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%s", port))
		if err != nil {
			t.Errorf("Error connecing to port '%s': %s", port, err)
		}
	}

	time.AfterFunc(time.Second, sd.stop)
	<-doneCh

	log.Printf("Checking ports...")

	patterns := make([]string, len(ports))
	for i, port := range ports {
		patterns[i] = fmt.Sprintf(`%s=\d+`, port)
	}
	pattern := regexp.MustCompile(strings.Join(patterns, ";"))

	if envPort := os.Getenv("SERVER_STARTER_PORT"); !pattern.MatchString(envPort) {
		t.Errorf("SERVER_STARTER_PORT: Expected '%s', but got '%s'", pattern, envPort)
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

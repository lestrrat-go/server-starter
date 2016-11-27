package starter

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"os"
	"os/exec"
	"path/filepath"
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
	"github.com/lestrrat/go-server-starter/listener"
)

func main() {
	listeners, err := listener.ListenAll()
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
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dir, err := ioutil.TempDir("", fmt.Sprintf("server-starter-test-%d", os.Getpid()))
	if err != nil {
		t.Errorf("Failed to create temp directory: %s", err)
		return
	}
	defer os.RemoveAll(dir)

	srcFile := filepath.Join(dir, "echod.go")
	f, err := os.OpenFile(srcFile, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0666)
	if err != nil {
		t.Errorf("Failed to create %s: %s", srcFile, err)
		return
	}
	io.WriteString(f, echoServerTxt)
	f.Close()

	cmd := exec.Command("go", "build", "-o", filepath.Join(dir, "echod"), ".")
	cmd.Dir = dir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Errorf("Failed to compile %s: %s\n%s", dir, err, output)
		return
	}

	ports := []string{"9090", "8080"}
	sd := New(filepath.Join(dir, "echod"), WithPorts(ports))
	go sd.Run(ctx)

	tick := time.NewTicker(500 * time.Millisecond)
	defer tick.Stop()

	ctx, cancel2 := context.WithTimeout(ctx, 5*time.Second)
	defer cancel2()
	for loop := true; loop; {
		select {
		case <-ctx.Done():
			t.Errorf("Error connecing: %s", ctx.Err())
			return
		case <-tick.C:
			ok := 0
			for _, port := range ports {
				_, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%s", port))
				if err == nil {
					t.Logf("Successfully connected to port %s", port)
					ok++
				}
			}
			if ok == len(ports) {
				loop = false
			}
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

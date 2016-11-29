package starter

import (
	"bytes"
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

	"github.com/stretchr/testify/assert"
)

var echoServerTxt = `package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"github.com/lestrrat/go-server-starter/listener"
)

func main() {
	listeners, err := listener.ListenAll()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to listen: %s\n", err)
		os.Exit(1)
	}
	defer func() {
		for _, l := range listeners {
			l.Close()
		}
	}()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(w, r.Body)
	})
	for _, l := range listeners {
		http.Serve(l, handler)
	}

	sigCh := make(chan os.Signal)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGHUP)
	select {
	case <-sigCh:
	}
}
`

func TestRun(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	dir, err := ioutil.TempDir("", fmt.Sprintf("server-starter-test-%d", os.Getpid()))
	if !assert.NoError(t, err, "failed to create tempdir %s", dir) {
		return
	}
	defer os.RemoveAll(dir)

	srcFile := filepath.Join(dir, "echod.go")
	f, err := os.OpenFile(srcFile, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0666)
	if !assert.NoError(t, err, "failed to create source file %s", f) {
		return
	}
	io.WriteString(f, echoServerTxt)
	f.Close()

	cmd := exec.Command("go", "build", "-o", filepath.Join(dir, "echod"), ".")
	cmd.Dir = dir
	if output, err := cmd.CombinedOutput(); !assert.NoError(t, err, "failed to compile echod") {
		t.Logf("%s", output)
		return
	}

	ports := []string{"9090", "8080"}
	var output bytes.Buffer
	sd := New(filepath.Join(dir, "echod"), WithPorts(ports), WithNoticeOutput(&output))

	done := make(chan struct{})
	go func() {
		defer close(done)
		if !assert.NoError(t, sd.Run(ctx), "Run should exit with no errors") {
			return
		}
	}()

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

	time.Sleep(time.Second)

	var closed bool
	select {
	case <-done:
		// grr, if we got here, done is closed
		closed = true
	default:
	}

	if !closed {
		p, _ := os.FindProcess(os.Getpid())
		p.Signal(os.Signal(syscall.SIGTERM))
	}

	<-done

	t.Logf("%s", output.String())
}

func TestSigFromName(t *testing.T) {
	for sig, name := range niceSigNames {
		if got := sigFromName(name); sig != got {
			t.Errorf("%v: wants '%v' but got '%v'", name, sig, got)
		}
	}

	variants := map[string]syscall.Signal{
		"SIGTERM": syscall.SIGTERM,
		"sigterm": syscall.SIGTERM,
		"Hup":     syscall.SIGHUP,
	}
	for name, sig := range variants {
		if got := sigFromName(name); sig != got {
			t.Errorf("%v: wants '%v' but got '%v'", name, sig, got)
		}
	}
}

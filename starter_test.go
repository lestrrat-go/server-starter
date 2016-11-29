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
	"strconv"
	"syscall"
	"testing"
	"time"

	"github.com/lestrrat/go-server-starter/internal/env"
	tcputil "github.com/lestrrat/go-tcputil"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
)

var echoServerSrc = `package main

import (
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"github.com/lestrrat/go-server-starter/listener"
)

func main() {
	fmt.Fprintf(os.Stderr, "Starting echod (%d)\n", os.Getpid())
	var maxSigusr1 int // number of times we "withstand" a sigusr1
	flag.IntVar(&maxSigusr1, "sigusr1", 0, "")
	flag.Parse()

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
		go http.Serve(l, handler)
	}

	fmt.Fprintf(os.Stderr, "echod: Waiting for signal (max USR1 = %d)\n", maxSigusr1)
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGUSR1)

	sigusr1 := 0
	for loop := true; loop; {
		select {
		case s := <-sigCh:
			fmt.Fprintf(os.Stderr, "echod: received %s\n", s)
			switch s {
			case syscall.SIGUSR1:
				sigusr1++
				if maxSigusr1 > sigusr1 {
					fmt.Fprintf(os.Stderr, "echod: got USR1, ignoring (max = %d, count = %d)\n", maxSigusr1, sigusr1)
				} else {
					fmt.Fprintf(os.Stderr, "echod: reached max USR1 limit (%d)\n", maxSigusr1)
					loop = false
				}
			case syscall.SIGTERM:
				loop = false
			default:
				// do nothing
			}
		}
	}
}
`

func build(name string, src string) (string, func(), error) {
	dir, err := ioutil.TempDir("", fmt.Sprintf("server-starter-test-%s-%d", name, os.Getpid()))
	if err != nil {
		return "", nil, errors.Wrapf(err, "failed to create tempdir %s", dir)
	}
	cleanup := func() {
		os.RemoveAll(dir)
	}
	srcFile := filepath.Join(dir, fmt.Sprintf("%s.go", name))
	f, err := os.OpenFile(srcFile, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0666)
	if err != nil {
		return "", cleanup, errors.Wrapf(err, "failed to create source file %s", f)
	}
	io.WriteString(f, src)
	f.Close()

	result := filepath.Join(dir, name)
	cmd := exec.Command("go", "build", "-o", result, ".")
	cmd.Env = nil
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", cleanup, errors.Wrapf(err, "failed to compile %s: %s", name, output)
	}
	return result, cleanup, nil
}

func TestRun(t *testing.T) {
	l := env.NewLoader()
	sysenv := env.SystemEnvironment()
	defer l.Restore(context.Background(), sysenv)

	cmdname, cleanup, err := build("echod", echoServerSrc)
	if cleanup != nil {
		defer cleanup()
	}
	if !assert.NoError(t, err, "build failed") {
		return
	}

	t.Run("normal execution", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		portCount := 2
		ports := make([]string, portCount)
		for i := 0; i < 2; i++ {
			p, err := tcputil.EmptyPort()
			if !assert.NoError(t, err, "failed to find an empty port") {
				return
			}
			ports[i] = strconv.Itoa(p)
		}
		var output bytes.Buffer
		defer func() {
			t.Logf("%s", output.String())
		}()
		sd := New(cmdname, WithPorts(ports), WithNoticeOutput(&output))

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
	})
	l.Restore(context.Background(), sysenv)

	t.Run("send multiple signals", func(t *testing.T) {
		// Note: this test does NOT test that the same echod server has received
		// signals, because doing that intelligently would require rpc between
		// this test code and the echod, and I really am in no mood to do it
		// for now. However, visually it looks like it's doing the right job
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()

		port, err := tcputil.EmptyPort()
		if !assert.NoError(t, err, "failed to find an empty port") {
			return
		}

		var output bytes.Buffer
		defer func() {
			t.Logf("%s", output.String())
		}()
		sd := New(cmdname,
			WithArgs("--sigusr1=2"),
			WithPorts([]string{strconv.Itoa(port)}),
			WithNoticeOutput(&output),
			WithLogStdout(&output),
			WithLogStderr(&output),
			WithSignalOnHUP(syscall.SIGUSR1),
		)

		done := make(chan struct{})
		go func() {
			defer close(done)
			if !assert.NoError(t, sd.Run(ctx), "Run should exit with no errors") {
				return
			}
		}()

		time.Sleep(2 * time.Second)

		var closed bool
		select {
		case <-done:
			closed = true
			t.Errorf("unexpected exit")
		default:
		}

		if !closed {
			p, _ := os.FindProcess(os.Getpid())
			p.Signal(syscall.SIGHUP)
		}

		time.Sleep(2 * time.Second)

		closed = false
		select {
		case <-done:
			closed = true
			t.Errorf("unexpected exit")
		default:
		}

		if !closed {
			p, _ := os.FindProcess(os.Getpid())
			p.Signal(syscall.SIGHUP)
		}

		time.Sleep(2 * time.Second)

		closed = false
		select {
		case <-done:
			closed = true
			t.Errorf("unexpected exit")
		default:
		}

		if !closed {
			p, _ := os.FindProcess(os.Getpid())
			p.Signal(syscall.SIGTERM)
		}

		time.Sleep(time.Second)

		select {
		case <-ctx.Done():
			t.Errorf("context prematurely ended: %s", ctx.Err())
			p, _ := os.FindProcess(os.Getpid())
			p.Signal(syscall.SIGTERM)
		case <-done:
		}
	})
	l.Restore(context.Background(), sysenv)
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

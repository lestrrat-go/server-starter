package server_starter

import (
	"fmt"
	"io"
	"io/ioutil"
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

func TestRun(t *testing.T) {
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

	ports := []int{9090, 8080}
	sd := Starter{
		ports:        ports,
		listeners:    make([]net.Listener, len(ports)),
		signalOnTERM: syscall.SIGTERM,
		Command:      filepath.Join(dir, "echod"),
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
		_, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", port))
		if err != nil {
			t.Errorf("Error connecing to port '%d': %s", port, err)
		}
	}

	time.AfterFunc(time.Second, sd.Stop)
	<-doneCh

	log.Printf("Checking ports...")

	patterns := make([]string, len(ports))
	for i, port := range ports {
		patterns[i] = fmt.Sprintf(`%d=\d+`, port)
	}
	pattern := regexp.MustCompile(strings.Join(patterns, ";"))

	if envPort := os.Getenv("SERVER_STARTER_PORT"); !pattern.MatchString(envPort) {
		t.Errorf("SERVER_STARTER_PORT: Expected '%s', but got '%s'", pattern, envPort)
	}

}

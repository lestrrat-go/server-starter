package server_starter

import (
	"fmt"
	"log"
	"net"
	"os"
//	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

func TestRun(t *testing.T) {
/*
	dir, err := ioutil.TempDir("", fmt.Sprintf("server-starter-test-%d", os.GetPid()))
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
	defer f.Close()
*/

	ports := []int{9090, 8080}
	sd := SuperDaemon{
		ports:     ports,
		listeners: make([]net.Listener, len(ports)),
		files:     make([]*os.File, len(ports)),
		stopCh:    make(chan struct{}),
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

package main

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

//
func TestShouldNotStartDuplicate(t *testing.T) {
	dir, err := ioutil.TempDir("", fmt.Sprintf("start-server-test-%d", os.Getpid()))
	if err != nil {
		t.Errorf("Failed to create temp directory: %s", err)
		return
	}
	defer os.RemoveAll(dir)

	pidFile := filepath.Join(dir, "start_server.pid")
	f, err := os.OpenFile(pidFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		t.Fatal("Failed to create pid file")
	}
	io.WriteString(f, "1111")
	f.Close()

	exeFile := filepath.Join(dir, "start_server")
	exec.Command("go", "build", "-o", exeFile).Run()

	stdout := new(bytes.Buffer)
	cmd := exec.Command(exeFile, "--pid-file="+pidFile, "hoge")
	cmd.Stdout = stdout
	cmd.Run()

	if !fileExist(pidFile) {
		t.Error("Missing pid file")
	}
	if stdout.String() != "pid file exists. already boot?" {
		t.Error("Find other error")
	}
}

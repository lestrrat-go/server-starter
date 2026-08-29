//go:build !windows

package supervisor

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const failingReplacementWorkerTxt = `package main

import (
	"os"
	"os/signal"
	"strconv"
	"syscall"
)

func main() {
	marker := os.Args[1]
	generation, _ := strconv.Atoi(os.Getenv("SERVER_STARTER_GENERATION"))
	if generation != 1 {
		_ = os.WriteFile(marker, []byte("started"), 0600)
		os.Exit(1)
	}

	term := make(chan os.Signal, 1)
	signal.Notify(term, syscall.SIGTERM)
	<-term
}
`

func buildFailingReplacementWorker(t *testing.T, dir string) string {
	t.Helper()

	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(failingReplacementWorkerTxt), 0600); err != nil {
		t.Fatalf("failed to write worker source: %s", err)
	}
	goMod := "module server-starter-failing-replacement\n\ngo 1.23\n"
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(goMod), 0600); err != nil {
		t.Fatalf("failed to write go.mod: %s", err)
	}

	bin := filepath.Join(dir, "failing-replacement")
	cmd := exec.CommandContext(context.Background(), "go", "build", "-buildvcs=false", "-o", bin, ".")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to compile worker: %s\n%s", err, output)
	}
	return bin
}

func waitForFile(t *testing.T, path string) {
	t.Helper()

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", path)
}

func TestRunRetriesInitialWorkerStartError(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "missing")
	marker := filepath.Join(root, "started")
	var stderr syncBuffer
	sd, err := NewStarter(&config{
		command: "/bin/sh",
		args:    []string{"-c", `printf started > "$1"; exec sleep 30`, "worker", marker},
		dir:     dir,
		stderr:  &stderr,
	})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ctrl, err := sd.Run(ctx)
	require.NoError(t, err)
	require.NotNil(t, ctrl)
	require.Eventually(t, func() bool {
		return strings.Contains(stderr.String(), "failed to exec")
	}, 10*time.Second, 20*time.Millisecond)
	require.NoError(t, os.Mkdir(dir, 0700))
	waitForFile(t, marker)
	cancel()
	require.ErrorIs(t, ctrl.Wait(), ErrServerClosed)
}

func TestRunWithStartupCheckReturnsInitialWorkerStartError(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "missing")
	sd, err := NewStarter(&config{
		command: "/bin/sh",
		args:    []string{"-c", "exec sleep 30"},
		dir:     dir,
	})
	require.NoError(t, err)

	ctrl, err := sd.RunWithStartupCheck(context.Background())
	require.Nil(t, ctrl)
	var pathErr *os.PathError
	require.ErrorAs(t, err, &pathErr)
	require.Equal(t, "chdir", pathErr.Op)
	require.Equal(t, dir, pathErr.Path)
	require.ErrorIs(t, pathErr.Err, fs.ErrNotExist)
}

func TestStartWorkerReturnsListenerDescriptorError(t *testing.T) {
	var lc net.ListenConfig
	bound, err := lc.Listen(context.Background(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)
	tcpListener := bound.(*net.TCPListener)
	require.NoError(t, tcpListener.Close())

	rs := &runState{
		cfg:         &Starter{command: "/bin/true", stderr: io.Discard},
		listeners:   []listener{{listener: tcpListener}},
		descriptors: []int{3},
	}
	_, err = rs.startWorker(context.Background(), make(chan processState), nil, make(chan error, 1))
	require.ErrorContains(t, err, "duplicate worker listener descriptor")
	require.ErrorIs(t, err, net.ErrClosed)
}

func TestSIGTERMDuringFailedReplacementDoesNotPanic(t *testing.T) {
	dir := t.TempDir()
	statusFile := filepath.Join(dir, "status")
	marker := filepath.Join(dir, "replacement-started")

	sd, err := NewStarter(&config{
		args:       []string{marker},
		command:    buildFailingReplacementWorker(t, dir),
		interval:   1,
		statusfile: statusFile,
	})
	if err != nil {
		t.Fatalf("failed to create starter: %s", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ctrl, err := sd.Run(ctx)
	if err != nil {
		t.Fatalf("sd.Run() failed: %s", err)
	}
	defer func() {
		cancel()
		select {
		case <-ctrl.Done():
		case <-time.After(10 * time.Second):
			t.Errorf("timed out waiting for Run() to return")
		}
	}()

	waitForGenerations(t, statusFile, 1)
	ctrl.Hangup()
	waitForFile(t, marker)
	cancel()

	select {
	case <-ctrl.Done():
	case <-time.After(10 * time.Second):
		t.Fatalf("timed out waiting for Run() to return")
	}
	if err := ctrl.Err(); err != nil && !errors.Is(err, ErrServerClosed) {
		t.Errorf("sd.Run() failed: %s", err)
	}
}

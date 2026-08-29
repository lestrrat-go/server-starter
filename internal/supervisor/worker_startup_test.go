//go:build !windows

package supervisor

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
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

func TestStartWorkerReturnsCancellationBeforeFirstAttempt(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := (&runState{}).startWorker(ctx, make(chan processState), nil, false)
	require.ErrorIs(t, err, context.Canceled)
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

func TestRunWithStartupCheckReturnsInitialWorkerExitError(t *testing.T) {
	dir := t.TempDir()
	firstAttempt := filepath.Join(dir, "first-attempt")
	retried := filepath.Join(dir, "retried")
	sd, err := NewStarter(&config{
		command: "/bin/sh",
		args: []string{"-c", `
			if [ -e "$1" ]; then
				printf retried > "$2"
				exec sleep 30
			fi
			: > "$1"
			exit 7
		`, testWorkerCommandName, firstAttempt, retried},
		interval: 1,
		stderr:   io.Discard,
	})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctrl, err := sd.RunWithStartupCheck(ctx)
	if ctrl != nil {
		cancel()
		require.ErrorIs(t, ctrl.Wait(), ErrServerClosed)
	}

	require.Nil(t, ctrl)
	require.ErrorContains(t, err, "exited before passing startup check")
	require.ErrorContains(t, err, fmt.Sprintf("status:%d", syscall.WaitStatus(7<<8)))
	require.NoFileExists(t, retried)
}

func TestRunWithStartupCheckRejectsImmediateExitAtZeroInterval(t *testing.T) {
	sd, err := NewStarter(&config{
		command:  testShellPath,
		args:     []string{"-c", "sleep 0.1; exit 7"},
		interval: 0,
		stderr:   io.Discard,
	})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	ctrl, err := sd.RunWithStartupCheck(ctx)
	if ctrl != nil {
		cancel()
		require.ErrorIs(t, ctrl.Wait(), ErrServerClosed)
	} else {
		cancel()
	}
	require.Nil(t, ctrl)
	require.ErrorContains(t, err, "exited before passing startup check")
	require.ErrorContains(t, err, fmt.Sprintf("status:%d", syscall.WaitStatus(7<<8)))
}

func TestRunWithStartupCheckReportsProbeCancellation(t *testing.T) {
	for _, interval := range []int{0, 30} {
		t.Run(fmt.Sprintf("interval %d", interval), func(t *testing.T) {
			marker := filepath.Join(t.TempDir(), "started")
			sd, err := NewStarter(&config{
				command:  testShellPath,
				args:     []string{"-c", `: > "$1"; exec sleep 30`, testWorkerCommandName, marker},
				interval: interval,
				stderr:   io.Discard,
			})
			require.NoError(t, err)

			ctx, cancel := context.WithCancel(context.Background())
			type runResult struct {
				ctrl *Controller
				err  error
			}
			resultCh := make(chan runResult, 1)
			go func() {
				ctrl, runErr := sd.RunWithStartupCheck(ctx)
				resultCh <- runResult{ctrl: ctrl, err: runErr}
			}()

			waitForFile(t, marker)
			cancel()

			var result runResult
			select {
			case result = <-resultCh:
			case <-time.After(3 * time.Second):
				t.Fatal("RunWithStartupCheck did not report probe cancellation")
			}
			if result.ctrl != nil {
				select {
				case <-result.ctrl.Done():
				case <-time.After(3 * time.Second):
					t.Fatal("successful controller did not stop after cancellation")
				}
			}
			require.Nil(t, result.ctrl)
			require.ErrorIs(t, result.err, context.Canceled)
		})
	}
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
	_, err = rs.startWorker(context.Background(), make(chan processState), nil, true)
	require.ErrorContains(t, err, "duplicate worker listener descriptor")
	require.ErrorIs(t, err, net.ErrClosed)
}

func TestBuildWorkerEnvOmitsDaemonReadinessDescriptor(t *testing.T) {
	t.Setenv("SERVER_STARTER_DAEMON_READY_FD", "9")

	for _, entry := range buildWorkerEnv(nil, "", 1) {
		require.NotEqual(t, "SERVER_STARTER_DAEMON_READY_FD", strings.SplitN(entry, "=", 2)[0])
	}
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

//go:build !windows

package supervisor

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// removedPlaceholder is the old literal placeholder that used to appear in
// num_old_workers= diagnostics, spelled out via concatenation so grepping
// this package's source for that placeholder finds no leftover occurrence.
var removedPlaceholder = "TO" + "DO"

// syncBuffer is a concurrency-safe io.Writer, needed wherever a test polls a
// buffer's contents while the supervisor's loop goroutine may still be
// writing diagnostics into it. A plain bytes.Buffer is not safe for that:
// its Write and the poll's read would race.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// TestReportFailedStartMessage covers worker.go's "new worker %d seems to
// have failed to start" diagnostic (issue #22).
//
// reportFailedStart is exercised directly, with the reaped-status and
// ProcessState-status inputs supplied by hand, rather than through a full
// supervisor Run(): TestFailedStartReportsReapedExitStatus below already
// covers the real end-to-end path (the one that actually happens on Unix,
// where findWorker's own WNOHANG wait4() reaps a worker that dies within
// the startup interval, consuming the exit status before startWorker's
// later cmd.Wait() can collect it -- see the doc comment on findWorker in
// worker_unix.go and on reportFailedStart in worker.go).
//
// A real *os.ProcessState is used for the ProcessState-status case rather
// than a fabricated one -- the type has no public constructor, so the only
// way to get one is to actually run a process to completion.
func TestReportFailedStartMessage(t *testing.T) {
	t.Run("with a reaped status", func(t *testing.T) {
		var buf bytes.Buffer
		reportFailedStart(&buf, 4242, syscall.WaitStatus(3<<8), true, nil)

		got := buf.String()
		require.Contains(t, got, "new worker 4242 seems to have failed to start")
		require.Contains(t, got, "status:")
		require.NotContains(t, got, removedPlaceholder)
	})

	t.Run("with a process state", func(t *testing.T) {
		cmd := exec.CommandContext(context.Background(), "/bin/sh", "-c", "exit 3")
		require.Error(t, cmd.Run(), "the command must exit non-zero")
		require.NotNil(t, cmd.ProcessState)

		var buf bytes.Buffer
		reportFailedStart(&buf, 4242, syscall.WaitStatus(0), false, cmd.ProcessState)

		got := buf.String()
		require.Contains(t, got, "new worker 4242 seems to have failed to start")
		require.Contains(t, got, "status:")
		require.NotContains(t, got, removedPlaceholder)
	})

	t.Run("without a status", func(t *testing.T) {
		var buf bytes.Buffer
		require.NotPanics(t, func() {
			reportFailedStart(&buf, 4242, syscall.WaitStatus(0), false, nil)
		})
		require.Equal(t, "new worker 4242 seems to have failed to start\n", buf.String())
	})
}

// TestFailedStartReportsReapedExitStatus covers the real end-to-end path on
// Unix: a worker that exits non-zero before the startup interval elapses is
// reaped by findWorker's own liveness probe, so the exit status must come
// from findWorker's reaped return value, not from cmd.ProcessState (which
// stays nil in this case -- see the doc comment on findWorker in
// worker_unix.go). Before the fix, this diagnostic never carried a status
// on Unix.
func TestFailedStartReportsReapedExitStatus(t *testing.T) {
	var buf syncBuffer
	sd, err := NewStarter(&config{
		command:  "/bin/false",
		interval: 1,
		stderr:   &buf,
	})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ctrl, err := sd.Run(ctx)
	require.NoError(t, err)

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) && !strings.Contains(buf.String(), "seems to have failed to start") {
		time.Sleep(20 * time.Millisecond)
	}

	cancel()
	select {
	case <-ctrl.Done():
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for Run() to return")
	}

	got := buf.String()
	require.Contains(t, got, "seems to have failed to start, status:")
}

// TestHangupReportsRealOldWorkerCount covers supervisor.go's two
// "num_old_workers=" diagnostics: both must report the real count of old
// workers, never the old literal placeholder (see removedPlaceholder).
func TestHangupReportsRealOldWorkerCount(t *testing.T) {
	dir := t.TempDir()
	statusFile := filepath.Join(dir, "status")

	// syncBuffer, not bytes.Buffer: the hangup leaves the old worker alive
	// alongside the new one, so their two cmd.Stderr pipes are drained by
	// os/exec's own goroutines concurrently, both writing into this buffer.
	var buf syncBuffer
	sd, err := NewStarter(&config{
		command:    buildStubbornWorker(t, dir),
		statusfile: statusFile,
		// The worker ignores USR1, so the old worker survives the hangup
		// and stays in rs.oldWorkers, giving num_old_workers something
		// other than zero to report.
		sigonhup: "USR1",
		stderr:   &buf,
	})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ctrl, err := sd.Run(ctx)
	require.NoError(t, err)

	waitForGenerations(t, statusFile, 1)
	ctrl.Hangup()
	waitForGenerations(t, statusFile, 2)

	cancel()
	select {
	case <-ctrl.Done():
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for Run() to return")
	}

	got := buf.String()
	require.NotContains(t, got, removedPlaceholder)
	require.Regexp(t, `num_old_workers=\d+`, got)
}

// TestConfiguredWriterReceivesSupervisorDiagnostics proves the supervisor's
// own diagnostics (not just the worker's stdout/stderr) go to the writer
// configured via Config.Stderr, and never leak to the process-level
// os.Stderr. Item 3 replaced a global os.Stderr = f reassignment with
// injected writers specifically so this is checkable at all; without this
// test, a future change could silently route the supervisor's own
// diagnostics back to the process-level stderr and nothing would catch it.
func TestConfiguredWriterReceivesSupervisorDiagnostics(t *testing.T) {
	r, w, err := os.Pipe()
	require.NoError(t, err)
	origStderr := os.Stderr
	os.Stderr = w
	restored := false
	defer func() {
		if !restored {
			os.Stderr = origStderr
		}
		r.Close()
		w.Close()
	}()

	var buf syncBuffer
	sd, err := NewStarter(&config{
		command: "/bin/sh",
		args:    []string{"-c", "exec sleep 30"},
		ports:   []string{"0"},
		stderr:  &buf,
	})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ctrl, err := sd.Run(ctx)
	require.NoError(t, err)

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) && !strings.Contains(buf.String(), "starting new worker") {
		time.Sleep(20 * time.Millisecond)
	}
	require.Contains(t, buf.String(), "starting new worker")

	cancel()
	select {
	case <-ctrl.Done():
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for Run() to return")
	}

	os.Stderr = origStderr
	restored = true
	require.NoError(t, w.Close())
	leaked, err := io.ReadAll(r)
	require.NoError(t, err)
	require.Empty(t, leaked, "supervisor diagnostics must not leak to the process-level stderr")
}

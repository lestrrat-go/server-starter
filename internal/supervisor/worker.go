package supervisor

import (
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"

	starter "github.com/lestrrat-go/server-starter/v2"
)

var successStatus syscall.WaitStatus
var failureStatus syscall.WaitStatus

func grabExitStatus(w io.Writer, st processState) syscall.WaitStatus {
	// Note: POSSIBLY non portable. seems to work on Unix/Windows
	// When/if this blows up, we will look for a cure
	exitSt, ok := st.Sys().(syscall.WaitStatus)
	if !ok {
		fmt.Fprintf(w, "Oh no, you are running on a platform where ProcessState.Sys().(syscall.WaitStatus) doesn't work! We're doomed! Temporarily setting status to 255. Please contact the author about this\n")
		exitSt = failureStatus
	}
	return exitSt
}

type processState interface {
	Pid() int
	Sys() any
}
type dummyProcessState struct {
	pid    int
	status syscall.WaitStatus
}

func (d dummyProcessState) Pid() int {
	return d.pid
}

func (d dummyProcessState) Sys() any {
	return d.status
}

// reportFailedStart writes the "worker failed to start" diagnostic to w.
//
// The status can come from two places, tried in this order:
//
//  1. reapedStatus, when reapedOK is true. On Unix, findWorker's liveness
//     probe reaps a worker that has already died as a side effect of
//     checking it, consuming its exit status before anything else (in
//     particular the caller's cmd.Wait()) can collect it. That reap is the
//     only place this status is ever observable, so the caller passes it
//     through here.
//  2. ps, when reapedOK is false and ps is non-nil. This is the path that
//     covers platforms without the reap race (Windows), where cmd.Wait()
//     still collects the exit status normally.
//
// ps is nil when the spawn itself failed (exec never produced a process, so
// there is no exit status to report) or when the worker vanished before its
// exit status could be collected; either way, dereferencing it would panic.
// When neither source has a status, the message falls back to the pid
// alone.
func reportFailedStart(w io.Writer, pid int, reapedStatus syscall.WaitStatus, reapedOK bool, ps *os.ProcessState) {
	switch {
	case reapedOK:
		fmt.Fprintf(w, "new worker %d seems to have failed to start, status:%d\n", pid, reapedStatus)
	case ps != nil:
		fmt.Fprintf(w, "new worker %d seems to have failed to start, status:%d\n", pid, grabExitStatus(w, ps))
	default:
		fmt.Fprintf(w, "new worker %d seems to have failed to start\n", pid)
	}
}

// startWorker starts the actual command. It returns a non-nil error only
// when its non-empty listener set cannot be formatted into a valid
// SERVER_STARTER_PORT spec (see starter.FormatPorts); that is a permanent
// misconfiguration, not a transient exec failure, so it is reported instead
// of retried.
func (rs *runState) startWorker(
	ctx context.Context,
	ch chan<- processState,
	done <-chan struct{},
) (*os.Process, error) {
	// Don't give up until we're running.
	for {
		pid := -1
		// reapedStatus/reapedOK carry findWorker's reaped exit status (set
		// below, once the worker has actually been started) out to the
		// reportFailedStart call after this block: on Unix, that reap is
		// the only place the status is ever observable. See findWorker's
		// doc comment in worker_unix.go.
		var reapedStatus syscall.WaitStatus
		reapedOK := false
		// The supervisor owns worker termination: on shutdown it sends
		// signalOnTERM and drains, so context cancellation must not kill
		// workers out from under that. WithoutCancel keeps the call
		// context-aware without handing the child's lifetime to ctx.
		cmd := exec.CommandContext(context.WithoutCancel(ctx), rs.cfg.command, rs.cfg.args...)
		if rs.cfg.dir != "" {
			cmd.Dir = rs.cfg.dir
		}
		cmd.Stdout = rs.cfg.stdout
		cmd.Stderr = rs.cfg.stderr

		// This whole section here basically sets up the env
		// var and the file descriptors that are inherited by the
		// external process
		descriptors := rs.descriptors
		maxFD := 2
		for _, fd := range descriptors {
			if fd > maxFD {
				maxFD = fd
			}
		}
		files := make([]*os.File, maxFD-2)
		portList := make(starter.List, len(rs.listeners))
		var err error
		for slot := range files {
			files[slot], err = os.OpenFile(os.DevNull, os.O_RDONLY, 0)
			if err != nil {
				for _, file := range files {
					if file != nil {
						file.Close()
					}
				}
				panic(err)
			}
		}
		for i, l := range rs.listeners {
			// file descriptor numbers in ExtraFiles turn out to be
			// index + 3, so we can just hard code it
			var f *os.File
			switch listener := l.listener.(type) {
			case *net.TCPListener:
				f, err = listener.File()
			case *net.UnixListener:
				f, err = listener.File()
			default:
				if packet, ok := l.packet.(*net.UDPConn); ok {
					f, err = packet.File()
				} else {
					err = fmt.Errorf("unknown listener type")
				}
			}
			if err != nil {
				for _, file := range files {
					if file != nil {
						file.Close()
					}
				}
				panic(err)
			}
			files[descriptors[i]-3].Close()
			files[descriptors[i]-3] = f
			portList[i] = l.starterListener(descriptors[i])
		}
		cmd.ExtraFiles = files

		portSpec := ""
		if len(portList) > 0 {
			portSpec, err = starter.FormatPorts(portList...)
			if err != nil {
				for _, file := range files {
					if file != nil {
						file.Close()
					}
				}
				return nil, fmt.Errorf("failed to format listeners for worker: %w", err)
			}
		}

		rs.generation++
		cmd.Env = buildWorkerEnv(rs.envOverlay, portSpec, rs.generation)

		// Now start!
		startErr := cmd.Start()
		for _, f := range files {
			if f != nil {
				f.Close()
			}
		}
		if startErr != nil {
			fmt.Fprintf(rs.cfg.stderr, "failed to exec %s: %s\n", cmd.Path, startErr)
		} else {
			// Save pid...
			pid = cmd.Process.Pid
			fmt.Fprintf(rs.cfg.stderr, "starting new worker %d\n", pid)

			// Wait for interval before checking if the process is alive. A
			// cancelled ctx bails out early too: it means a shutdown was
			// requested, so there is no point waiting out the rest of the
			// interval before deciding the worker "started".
			tch := time.After(rs.cfg.interval)
			ctxDone := false
			select {
			case <-tch:
			case <-ctx.Done():
				ctxDone = true
			}

			// Check if we can find a process by its pid
			var p *os.Process
			p, reapedStatus, reapedOK = findWorker(pid)
			if ctxDone || p != nil {
				// No error? We were successful! Make sure we capture
				// the program exiting
				go func() {
					err := cmd.Wait()
					var st processState
					var exitErr *exec.ExitError
					if errors.As(err, &exitErr) {
						st = exitErr.ProcessState
					} else if err != nil {
						st = &dummyProcessState{pid: pid, status: failureStatus}
					} else {
						st = &dummyProcessState{pid: pid, status: successStatus}
					}
					select {
					case ch <- st:
					case <-done:
					}
				}()
				// Bail out
				return p, nil
			}
		}
		// If we fall through here, we prematurely exited :/
		// Make sure to wait to release resources
		_ = cmd.Wait()
		for _, f := range cmd.ExtraFiles {
			f.Close()
		}

		reportFailedStart(rs.cfg.stderr, pid, reapedStatus, reapedOK, cmd.ProcessState)
	}
}

// buildWorkerEnv builds the environment for a spawned worker explicitly,
// rather than relying on exec.Command's default inheritance combined with
// mutating the supervisor's own process environment (the old approach,
// which two concurrent supervisors running in the same process would race
// on). It
// starts from the supervisor's own environment, overlays the envdir map
// (last-loaded envdir wins over the ambient value, mirroring the old
// setEnv's precedence), then sets the protocol variables. Building a map
// first and flattening it afterward guarantees each key appears once in the
// result: a duplicate KEY= entry in cmd.Env is resolved by "last one wins"
// on Linux, but that is not something to depend on for readability.
func buildWorkerEnv(overlay map[string]string, portSpec string, generation int) []string {
	merged := make(map[string]string, len(overlay)+2)
	for _, kv := range os.Environ() {
		if k, v, ok := strings.Cut(kv, "="); ok {
			merged[k] = v
		}
	}
	maps.Copy(merged, overlay)
	merged[starter.PortEnvName] = portSpec
	merged[starter.GenerationEnvName] = strconv.Itoa(generation)

	env := make([]string, 0, len(merged))
	for k, v := range merged {
		env = append(env, k+"="+v)
	}
	return env
}

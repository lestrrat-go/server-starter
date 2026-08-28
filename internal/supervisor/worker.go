package supervisor

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

var successStatus syscall.WaitStatus
var failureStatus syscall.WaitStatus

func grabExitStatus(st processState) syscall.WaitStatus {
	// Note: POSSIBLY non portable. seems to work on Unix/Windows
	// When/if this blows up, we will look for a cure
	exitSt, ok := st.Sys().(syscall.WaitStatus)
	if !ok {
		fmt.Fprintf(os.Stderr, "Oh no, you are running on a platform where ProcessState.Sys().(syscall.WaitStatus) doesn't work! We're doomed! Temporarily setting status to 255. Please contact the author about this\n")
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

// startWorker starts the actual command.
func (rs *runState) startWorker(ctx context.Context, ch chan processState) *os.Process {
	// Don't give up until we're running.
	for {
		pid := -1
		cmd := exec.Command(rs.cfg.command, rs.cfg.args...)
		if rs.cfg.dir != "" {
			cmd.Dir = rs.cfg.dir
		}
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		// This whole section here basically sets up the env
		// var and the file descriptors that are inherited by the
		// external process
		descriptors := make([]int, len(rs.listeners))
		used := make(map[int]struct{}, len(rs.listeners))
		for i, l := range rs.listeners {
			if l.fd >= 0 {
				descriptors[i] = l.fd
				used[l.fd] = struct{}{}
			}
		}
		nextFD := 3
		for i := range descriptors {
			if descriptors[i] != 0 {
				continue
			}
			for {
				if _, ok := used[nextFD]; !ok {
					descriptors[i] = nextFD
					used[nextFD] = struct{}{}
					nextFD++
					break
				}
				nextFD++
			}
		}
		maxFD := 2
		for _, fd := range descriptors {
			if fd > maxFD {
				maxFD = fd
			}
		}
		files := make([]*os.File, maxFD-2)
		ports := make([]string, len(rs.listeners))
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
			ports[i] = fmt.Sprintf("%s=%d", l.spec, descriptors[i])
		}
		cmd.ExtraFiles = files

		rs.generation++
		os.Setenv("SERVER_STARTER_PORT", strings.Join(ports, ";"))
		os.Setenv("SERVER_STARTER_GENERATION", fmt.Sprintf("%d", rs.generation))

		// Now start!
		startErr := cmd.Start()
		for _, f := range files {
			if f != nil {
				f.Close()
			}
		}
		if startErr != nil {
			fmt.Fprintf(os.Stderr, "failed to exec %s: %s\n", cmd.Path, startErr)
		} else {
			// Save pid...
			pid = cmd.Process.Pid
			fmt.Fprintf(os.Stderr, "starting new worker %d\n", pid)

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
			p := findWorker(pid)
			if ctxDone || p != nil {
				// No error? We were successful! Make sure we capture
				// the program exiting
				go func() {
					err := cmd.Wait()
					var exitErr *exec.ExitError
					if errors.As(err, &exitErr) {
						ch <- exitErr.ProcessState
					} else if err != nil {
						ch <- &dummyProcessState{pid: pid, status: failureStatus}
					} else {
						ch <- &dummyProcessState{pid: pid, status: successStatus}
					}
				}()
				// Bail out
				return p
			}
		}
		// If we fall through here, we prematurely exited :/
		// Make sure to wait to release resources
		_ = cmd.Wait()
		for _, f := range cmd.ExtraFiles {
			f.Close()
		}

		fmt.Fprintf(os.Stderr, "new worker %d seems to have failed to start\n", pid)
	}
}

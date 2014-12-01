package server_starter

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

type SuperDaemon struct {
	interval     time.Duration
	signalOnHUP  os.Signal
	signalOnTERM os.Signal
	// you can't set this in go:	backlog
	statusFile string
	pidFile    string
	dir        string
	ports      []int
	paths      []string
	listeners  []net.Listener
	Command    string
	Args       []string
}

func (sd *SuperDaemon) Close() {
	if sd.statusFile != "" {
		os.Remove(sd.statusFile)
	}

	if sd.pidFile != "" {
		os.Remove(sd.pidFile)
	}
}

func (sd SuperDaemon) Stop() {
	fmt.Fprintf(os.Stderr, "Calling stop()\n")
	p, _ := os.FindProcess(os.Getpid())
	p.Signal(syscall.SIGTERM)
}

func grabExitStatus(st processState) syscall.WaitStatus {
	// Note: POSSIBLY non portable. seems to work on Unix/Windows
	// When/if this blows up, we will look for a cure
	exitSt, ok := st.Sys().(syscall.WaitStatus)
	if !ok {
		fmt.Fprintf(os.Stderr, "Oh no, you are running on a platform where ProcessState.Sys().(syscall.WaitStatus) doesn't work! We're doomed! Temporarily setting status to 255. Please contact the author about this\n")
		exitSt = syscall.WaitStatus(255)
	}
	return exitSt
}

type processState interface {
	Pid() int
	Sys() interface{}
}
type dummyProcessState struct {
	pid    int
	status syscall.WaitStatus
}

func (d dummyProcessState) Pid() int {
	return d.pid
}

func (d dummyProcessState) Sys() interface {} {
	return d.status
}

func (sd *SuperDaemon) Run() error {
	defer sd.Teardown()

	for i, port := range sd.ports {
		l, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
		if err != nil {
			return err
		}

		sd.listeners[i] = l
	}

	os.Setenv("SERVER_STARTER_GENERATION", "0")

	// XXX Not portable
	sigCh := make(chan os.Signal)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGHUP)

	// Okay, ready to launch the program now...
	workerCh := make(chan processState)
	p := sd.StartWorker(workerCh)

//	var lastRestartTime time.Time
	for { // outer loop
		_, err := reloadEnv()
		if err != nil {
			// do something
		}

		// Just wait for the worker to exit, or for us to receive a signal
		for {
			select {
			case st := <-workerCh:
				// oops, the worker exited? check for its pid
				if p.Pid == st.Pid() { // current worker
					exitSt := grabExitStatus(st)
					fmt.Fprintf(os.Stderr, "worker %d died unexpectedly with status %d, restarting\n", p.Pid, exitSt)
					p = sd.StartWorker(workerCh)
					// lastRestartTime = time.Now()
				} else {
					exitSt := grabExitStatus(st)
					fmt.Fprintf(os.Stderr, "old worker %d died, status:%d\n", st.Pid(), exitSt)

					// delete $old_workers{$died_worker}
				}
			case _ = <-sigCh:
				// Temporary fix
				p.Signal(syscall.SIGTERM)
				return nil
				/*
					switch sig {
					case syscall.SIGHUP:
					case syscall.SIGTERM:
						sd.terminate(sd.signalOnTERM)
					default:
						sd.terminate(syscall.SIGTERM)
						return nil
					}
				*/
			}
		}
	}

	return nil
}

func (sd *SuperDaemon) terminate(sig os.Signal) {

}

type WorkerState int

const (
	WorkerStarted WorkerState = iota
	ErrFailedToStart
)

// StartWorker starts the actual command.
func (sd *SuperDaemon) StartWorker(ch chan processState) *os.Process {
	// Don't give up until we're running.
	for {
		pid := -1
		cmd := exec.Command(sd.Command, sd.Args...)
		if sd.dir != "" {
			cmd.Dir = sd.dir
		}
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		files := make([]*os.File, len(sd.ports))
		ports := make([]string, len(sd.ports))
		for i, l := range sd.listeners {
			f, err := l.(*net.TCPListener).File()
			if err != nil {
				panic(err)
			}
			defer f.Close()

			// file descriptor numbers in ExtraFiles turn out to be
			// index + 3, so we can just hard code it
			ports[i] = fmt.Sprintf("%d=%d", sd.ports[i], i+3)
			files[i] = f
		}

		os.Setenv("SERVER_STARTER_PORT", strings.Join(ports, ";"))
		cmd.ExtraFiles = files

		if err := cmd.Start(); err != nil {
			fmt.Fprintf(os.Stderr, "failed to exec %s: %s\n", cmd.Path, err)
			goto FAILED_TO_START
		}

		// Save pid...
		pid = cmd.Process.Pid
		fmt.Fprintf(os.Stderr, "starting new worker %d\n", pid)

		// Wait for interval before checking if the process is alive
		time.Sleep(sd.interval)

		// XXX We punted this
		// if ((grep { $_ ne 'HUP' } @signals_received)

		// Check if we can find a process by its pid
		if p, err := os.FindProcess(pid); err == nil {
			// No error? We were successful! Make sure we capture
			// the program exiting
			go func() {
				err := cmd.Wait()
				if err != nil {
					ch <- err.(*exec.ExitError).ProcessState
				} else {
					ch <- &dummyProcessState{pid: pid, status: 0}
				}
			}()
			// Bail out
			return p
		}

		// If we fall through here, we prematurely exited :/
	FAILED_TO_START:
		// Make sure to wait to release resources
		cmd.Wait()

		fmt.Fprintf(os.Stderr, "new worker %d seems to have failed to start\n", pid)
	}

	// never reached
	return nil
}

func (sd *SuperDaemon) Teardown() error {
	for _, l := range sd.listeners {
		if l == nil {
			continue
		}
		l.Close()
	}

	return nil
}
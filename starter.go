package server_starter

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type Starter struct {
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
	generation int
	Command    string
	Args       []string
}

func (s *Starter) Close() {
	if s.statusFile != "" {
		os.Remove(s.statusFile)
	}

	if s.pidFile != "" {
		os.Remove(s.pidFile)
	}
}

func (s Starter) Stop() {
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

func (d dummyProcessState) Sys() interface{} {
	return d.status
}

var niceSigNames = map[syscall.Signal]string{
	syscall.SIGHUP:  "HUP",
	syscall.SIGINT:  "INT",
	syscall.SIGQUIT: "QUIT",
	syscall.SIGTERM: "TERM",
}

func signame(s os.Signal) string {
	if ss, ok := s.(syscall.Signal); ok {
		return niceSigNames[ss]
	}
	return "UNKNOWN"
}

func (s *Starter) Run() error {
	defer s.Teardown()

	for i, port := range s.ports {
		l, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
		if err != nil {
			return err
		}

		s.listeners[i] = l
	}

	s.generation = 0
	os.Setenv("SERVER_STARTER_GENERATION", fmt.Sprintf("%d", s.generation))

	// XXX Not portable
	sigCh := make(chan os.Signal)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGHUP)

	// Okay, ready to launch the program now...
	workerCh := make(chan processState)
	p := s.StartWorker(workerCh)
	oldWorkers := make(map[int]int)
	var sigReceived os.Signal
	var sigToSend os.Signal

	defer func() {
		oldWorkers[p.Pid] = s.generation

		fmt.Fprintf(os.Stderr, "received %s, sending %s to all workers:",
			signame(sigReceived),
			signame(sigToSend),
		)
		size := len(oldWorkers)
		i := 0
		for pid := range oldWorkers {
			i++
			fmt.Fprintf(os.Stderr, "%d", pid)
			if i < size {
				fmt.Fprintf(os.Stderr, ",")
			}
		}
		fmt.Fprintf(os.Stderr, "\n")

		for pid := range oldWorkers {
			worker, err := os.FindProcess(pid)
			if err != nil {
				continue
			}
			worker.Signal(sigToSend)
		}

		for len(oldWorkers) > 0 {
			st := <-workerCh
			fmt.Fprintf(os.Stderr, "worker %d died, status:%d\n", st.Pid(), grabExitStatus(st))
			delete(oldWorkers, st.Pid())
		}
		fmt.Fprintf(os.Stderr, "exiting\n")
	}()

	//	var lastRestartTime time.Time
	for { // outer loop
		_, err := reloadEnv()
		if err != nil {
			// do something
		}

		// Just wait for the worker to exit, or for us to receive a signal
		for {
			// restart = 2: force restart
			// restart = 1 and no workers: force restart
			// restart = 0: no restart
			restart := 0

			select {
			case st := <-workerCh:
				// oops, the worker exited? check for its pid
				if p.Pid == st.Pid() { // current worker
					exitSt := grabExitStatus(st)
					fmt.Fprintf(os.Stderr, "worker %d died unexpectedly with status %d, restarting\n", p.Pid, exitSt)
					p = s.StartWorker(workerCh)
					// lastRestartTime = time.Now()
				} else {
					exitSt := grabExitStatus(st)
					fmt.Fprintf(os.Stderr, "old worker %d died, status:%d\n", st.Pid(), exitSt)
					delete(oldWorkers, st.Pid())
				}
			case sigReceived = <-sigCh:
				// Temporary fix
				switch sigReceived {
				case syscall.SIGHUP:
					// When we receive a HUP signal, we need to spawn a new worker
					fmt.Fprintf(os.Stderr, "received HUP (num_old_workers=TODO)\n")
					restart = 1
				case syscall.SIGTERM:
					sigToSend = s.signalOnTERM
					return nil
				default:
					sigToSend = syscall.SIGTERM
					return nil
				}
			}

			if restart > 1 || restart > 0 && len(oldWorkers) == 0 {
				fmt.Fprintf(os.Stderr, "spawning a new worker (num_old_workers=TODO)\n")
				oldWorkers[p.Pid] = s.generation
				p = s.StartWorker(workerCh)
				fmt.Fprintf(os.Stderr, "new worker is now running, sending $opts->{signal_on_hup} to old workers:")
				size := len(oldWorkers)
				if size == 0 {
					fmt.Fprintf(os.Stderr, "none\n")
				} else {
					i := 0
					for pid := range oldWorkers {
						i++
						fmt.Fprintf(os.Stderr, "%d", pid)
						if i < size {
							fmt.Fprintf(os.Stderr, ",")
						}
					}
					fmt.Fprintf(os.Stderr, "\n")

					killOldDelay := getKillOldDelay()
					fmt.Fprintf(os.Stderr, "sleep %d secs\n", int(killOldDelay/time.Second))
					if killOldDelay > 0 {
						time.Sleep(killOldDelay)
					}

					fmt.Fprintf(os.Stderr, "killing old workers\n")

					for pid := range oldWorkers {
						worker, err := os.FindProcess(pid)
						if err != nil {
							continue
						}
						worker.Signal(s.signalOnHUP)
					}
				}
			}
		}
	}

	return nil
}

func getKillOldDelay() time.Duration {
	// Ignore errors.
	delay, _ := strconv.ParseInt(os.Getenv("KILL_OLD_DELAY"), 10, 0)
	autoRestart, _ := strconv.ParseBool(os.Getenv("ENABLE_AUTO_RESTART"))
	if autoRestart && delay == 0 {
		delay = 5
	}

	return time.Duration(delay) * time.Second
}

func (s *Starter) terminate(sig os.Signal) {

}

type WorkerState int

const (
	WorkerStarted WorkerState = iota
	ErrFailedToStart
)

// StartWorker starts the actual command.
func (s *Starter) StartWorker(ch chan processState) *os.Process {
	// Don't give up until we're running.
	for {
		pid := -1
		cmd := exec.Command(s.Command, s.Args...)
		if s.dir != "" {
			cmd.Dir = s.dir
		}
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		// This whole section here basically sets up the env
		// var and the file descriptors that are inherited by the
		// external process
		files := make([]*os.File, len(s.ports))
		ports := make([]string, len(s.ports))
		for i, l := range s.listeners {
			f, err := l.(*net.TCPListener).File()
			if err != nil {
				panic(err)
			}
			defer f.Close()

			// file descriptor numbers in ExtraFiles turn out to be
			// index + 3, so we can just hard code it
			ports[i] = fmt.Sprintf("%d=%d", s.ports[i], i+3)
			files[i] = f
		}
		cmd.ExtraFiles = files

		s.generation++
		os.Setenv("SERVER_STARTER_PORT", strings.Join(ports, ";"))
		os.Setenv("SERVER_STARTER_GENERATION", fmt.Sprintf("%d", s.generation))

		// Now start!
		if err := cmd.Start(); err != nil {
			fmt.Fprintf(os.Stderr, "failed to exec %s: %s\n", cmd.Path, err)
			goto FAILED_TO_START
		}

		// Save pid...
		pid = cmd.Process.Pid
		fmt.Fprintf(os.Stderr, "starting new worker %d\n", pid)

		// Wait for interval before checking if the process is alive
		time.Sleep(s.interval)

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

func (s *Starter) Teardown() error {
	for _, l := range s.listeners {
		if l == nil {
			continue
		}
		l.Close()
	}

	return nil
}
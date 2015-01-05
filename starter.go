package starter

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

var niceSigNames map[syscall.Signal]string
var niceNameToSigs map[string]syscall.Signal
var successStatus syscall.WaitStatus
var failureStatus syscall.WaitStatus

func makeNiceSigNamesCommon() map[syscall.Signal]string {
	return map[syscall.Signal]string{
		syscall.SIGABRT: "ABRT",
		syscall.SIGALRM: "ALRM",
		syscall.SIGBUS:  "BUS",
		// syscall.SIGEMT:  "EMT",
		syscall.SIGFPE: "FPE",
		syscall.SIGHUP: "HUP",
		syscall.SIGILL: "ILL",
		// syscall.SIGINFO: "INFO",
		syscall.SIGINT: "INT",
		// syscall.SIGIOT:    "IOT",
		syscall.SIGKILL: "KILL",
		syscall.SIGPIPE: "PIPE",
		syscall.SIGQUIT: "QUIT",
		syscall.SIGSEGV: "SEGV",
		syscall.SIGTERM: "TERM",
		syscall.SIGTRAP: "TRAP",
	}
}

func makeNiceSigNames() map[syscall.Signal]string {
	return addPlatformDependentNiceSigNames(makeNiceSigNamesCommon())
}

func init() {
	niceSigNames = makeNiceSigNames()
	niceNameToSigs := make(map[string]syscall.Signal)
	for sig, name := range niceSigNames {
		niceNameToSigs[name] = sig
	}
}

type Config interface {
	Args() []string
	Command() string
	Dir() string             // Dirctory to chdir to before executing the command
	Interval() time.Duration // Time between checks for liveness
	PidFile() string
	Ports() []string         // Ports to bind to (addr:port or port, so it's a string)
	Paths() []string         // Paths (UNIX domain socket) to bind to
	SignalOnHUP() os.Signal  // Signal to send when HUP is received
	SignalOnTERM() os.Signal // Signal to send when TERM is received
	StatusFile() string
}

type Starter struct {
	interval     time.Duration
	signalOnHUP  os.Signal
	signalOnTERM os.Signal
	// you can't set this in go:	backlog
	statusFile string
	pidFile    string
	dir        string
	ports      []string
	paths      []string
	listeners  []net.Listener
	generation int
	command    string
	args       []string
}

// NewStarter creates a new Starter object. Config parameter may NOT be
// nil, as `Ports` and/or `Paths`, and `Command` are required
func NewStarter(c Config) (*Starter, error) {
	if c == nil {
		return nil, fmt.Errorf("config argument must be non-nil")
	}

	var signalOnHUP os.Signal = syscall.SIGTERM
	var signalOnTERM os.Signal = syscall.SIGTERM
	if s := c.SignalOnHUP(); s != nil {
		signalOnHUP = s
	}
	if s := c.SignalOnTERM(); s != nil {
		signalOnTERM = s
	}

	if c.Command() == "" {
		return nil, fmt.Errorf("argument Command must be specified")
	}
	if _, err := exec.LookPath(c.Command()); err != nil {
		return nil, err
	}

	s := &Starter{
		args:         c.Args(),
		command:      c.Command(),
		dir:          c.Dir(),
		interval:     c.Interval(),
		listeners:    make([]net.Listener, len(c.Ports())+len(c.Paths())),
		pidFile:      c.PidFile(),
		ports:        c.Ports(),
		paths:        c.Paths(),
		signalOnHUP:  signalOnHUP,
		signalOnTERM: signalOnTERM,
		statusFile:   c.StatusFile(),
	}

	return s, nil

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
		exitSt = failureStatus
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

func signame(s os.Signal) string {
	if ss, ok := s.(syscall.Signal); ok {
		return niceSigNames[ss]
	}
	return "UNKNOWN"
}

func SigFromName(n string) os.Signal {
	if sig, ok := niceNameToSigs[n]; ok {
		return sig
	}
	return nil
}

func setEnv() {
	if os.Getenv("ENVDIR") == "" {
		return
	}

	m, err := reloadEnv()
	if err != nil {
		// do something
		fmt.Fprintf(os.Stderr, "failed to load from envdir: %s", err)
	}

	for k, v := range m {
		os.Setenv(k, v)
	}
}

func (s *Starter) Run() error {
	defer s.Teardown()

	if s.pidFile != "" {
		f, err := os.OpenFile(s.pidFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
		if err != nil {
			return err
		}

		fmt.Fprintf(f, "%d", os.Getpid())
		f.Close()
	}

	for i, addr := range s.ports {
		var l net.Listener
		port, err := strconv.ParseInt(addr, 10, 64)
		if err == nil { // Looks like port only
			l, err = net.Listen("tcp4", fmt.Sprintf(":%d", port))
			if err != nil {
				fmt.Fprintf(os.Stderr, "failed to listen to :%d:%s\n", port, err)
				return err
			}
		} else {
			l, err = net.Listen("tcp4", addr)
			if err != nil {
				fmt.Fprintf(os.Stderr, "failed to listen to %s:%s\n", addr, err)
				return err
			}
		}
		s.listeners[i] = l
	}

	for i, path := range s.paths {
		var l net.Listener
		if fl, err := os.Lstat(path); err == nil && fl.Mode()&os.ModeSocket == os.ModeSocket {
			fmt.Fprintf(os.Stderr, "removing existing socket file:%s\n", path)
			err = os.Remove(path)
			if err != nil {
				fmt.Fprintf(os.Stderr, "failed to remove existing socket file:%s:%s\n", path, err)
				return err
			}
		}
		_ = os.Remove(path)
		l, err := net.Listen("unix", path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to listen file:%s:%s\n", path, err)
			return err
		}
		s.listeners[i+len(s.ports)] = l
	}

	s.generation = 0
	os.Setenv("SERVER_STARTER_GENERATION", fmt.Sprintf("%d", s.generation))

	// XXX Not portable
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh,
		syscall.SIGHUP,
		syscall.SIGINT,
		syscall.SIGTERM,
		syscall.SIGQUIT,
	)

	// Okay, ready to launch the program now...
	setEnv()
	workerCh := make(chan processState)
	p := s.StartWorker(sigCh, workerCh)
	oldWorkers := make(map[int]int)
	var sigReceived os.Signal
	var sigToSend os.Signal

	statusCh := make(chan map[int]int)
	go func(fn string, ch chan map[int]int) {
		for wmap := range ch {
			if fn == "" {
				continue
			}

			f, err := os.OpenFile(fn, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
			if err != nil {
				continue
			}

			for gen, pid := range wmap {
				fmt.Fprintf(f, "%d:%d\n", gen, pid)
			}

			f.Close()
		}
	}(s.statusFile, statusCh)

	defer func() {
		if p != nil {
			oldWorkers[p.Pid] = s.generation
		}

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
		setEnv()

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
					p = s.StartWorker(sigCh, workerCh)
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
					sigToSend = s.signalOnHUP
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
				p = s.StartWorker(sigCh, workerCh)
				fmt.Fprintf(os.Stderr, "new worker is now running, sending %s to old workers:", signame(sigToSend))
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

type WorkerState int

const (
	WorkerStarted WorkerState = iota
	ErrFailedToStart
)

// StartWorker starts the actual command.
func (s *Starter) StartWorker(sigCh chan os.Signal, ch chan processState) *os.Process {
	// Don't give up until we're running.
	for {
		pid := -1
		cmd := exec.Command(s.command, s.args...)
		if s.dir != "" {
			cmd.Dir = s.dir
		}
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		// This whole section here basically sets up the env
		// var and the file descriptors that are inherited by the
		// external process
		files := make([]*os.File, len(s.ports)+len(s.paths))
		ports := make([]string, len(s.ports)+len(s.paths))
		for i, l := range s.listeners {
			// file descriptor numbers in ExtraFiles turn out to be
			// index + 3, so we can just hard code it
			if i < len(s.ports) {
				f, err := l.(*net.TCPListener).File()
				if err != nil {
					panic(err)
				}
				defer f.Close()
				ports[i] = fmt.Sprintf("%s=%d", s.ports[i], i+3)
				files[i] = f
			} else {
				f, err := l.(*net.UnixListener).File()
				if err != nil {
					panic(err)
				}
				defer f.Close()
				ports[i] = fmt.Sprintf("%s=%d", s.paths[i-len(s.ports)], i+3)
				files[i] = f
			}

		}
		cmd.ExtraFiles = files

		s.generation++
		os.Setenv("SERVER_STARTER_PORT", strings.Join(ports, ";"))
		os.Setenv("SERVER_STARTER_GENERATION", fmt.Sprintf("%d", s.generation))

		// Now start!
		if err := cmd.Start(); err != nil {
			fmt.Fprintf(os.Stderr, "failed to exec %s: %s\n", cmd.Path, err)
		} else {
			// Save pid...
			pid = cmd.Process.Pid
			fmt.Fprintf(os.Stderr, "starting new worker %d\n", pid)

			// Wait for interval before checking if the process is alive
			tch := time.After(s.interval)
			sigs := []os.Signal{}
			for loop := true; loop; {
				select {
				case <-tch:
					// bail out
					loop = false
				case sig := <-sigCh:
					sigs = append(sigs, sig)
				}
			}

			// if received any signals, during the wait, we bail out
			gotSig := false
			if len(sigs) > 0 {
				for _, sig := range sigs {
					// we need to resend these signals so it can be caught in the
					// main routine...
					go func() { sigCh <- sig }()
					if sysSig, ok := sig.(syscall.Signal); ok {
						if sysSig != syscall.SIGHUP {
							gotSig = true
						}
					}
				}
			}

			// Check if we can find a process by its pid
			p, err := os.FindProcess(pid)
			if gotSig || err == nil {
				// No error? We were successful! Make sure we capture
				// the program exiting
				go func() {
					err := cmd.Wait()
					if err != nil {
						ch <- err.(*exec.ExitError).ProcessState
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
		cmd.Wait()
		for _, f := range cmd.ExtraFiles {
			f.Close()
		}

		fmt.Fprintf(os.Stderr, "new worker %d seems to have failed to start\n", pid)
	}

	// never reached
	return nil
}

func (s *Starter) Teardown() error {
	if s.pidFile != "" {
		os.Remove(s.pidFile)
	}

	if s.statusFile != "" {
		os.Remove(s.statusFile)
	}

	for _, l := range s.listeners {
		if l == nil {
			continue
		}
		l.Close()
	}

	return nil
}

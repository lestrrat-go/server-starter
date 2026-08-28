package starter

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"sync"
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
	niceNameToSigs = make(map[string]syscall.Signal)
	for sig, name := range niceSigNames {
		niceNameToSigs[name] = sig
	}
}

type listener struct {
	listener net.Listener
	packet   net.PacketConn
	fd       int
	spec     string // path or port spec
}

type portTarget struct {
	host    string
	port    int
	network string
	spec    string
	fd      int
}

type Config interface {
	Args() []string
	Command() string
	Dir() string             // Directory to chdir to before executing the command
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
	listeners  []listener
	generation int
	command    string
	args       []string
	mu         sync.RWMutex
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
		listeners:    make([]listener, 0, len(c.Ports())+len(c.Paths())),
		pidFile:      c.PidFile(),
		ports:        c.Ports(),
		paths:        c.Paths(),
		signalOnHUP:  signalOnHUP,
		signalOnTERM: signalOnTERM,
		statusFile:   c.StatusFile(),
	}

	return s, nil
}

func (s *Starter) Stop() {
	p, _ := os.FindProcess(os.Getpid())
	_ = p.Signal(syscall.SIGTERM)
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

func signame(s os.Signal) string {
	if ss, ok := s.(syscall.Signal); ok {
		return niceSigNames[ss]
	}
	return "UNKNOWN"
}

// SigFromName returns the signal corresponding to the given signal name string.
// If the given name string is not defined, it returns nil.
func SigFromName(n string) os.Signal {
	n = strings.TrimPrefix(strings.ToUpper(n), "SIG")
	if sig, ok := niceNameToSigs[n]; ok {
		return sig
	}
	return nil
}

func parsePortTarget(raw string) (portTarget, error) {
	target := strings.TrimSpace(raw)
	fd := -1
	if i := strings.LastIndexByte(target, '='); i >= 0 {
		value, err := strconv.Atoi(strings.TrimSpace(target[i+1:]))
		if err != nil || value < 0 {
			return portTarget{}, fmt.Errorf("invalid file descriptor in %q", raw)
		}
		fd = value
		target = strings.TrimSpace(target[:i])
	}

	udp := strings.HasPrefix(target, "u")
	if udp {
		target = strings.TrimPrefix(target, "u")
	}
	host := ""
	portText := target
	if strings.HasPrefix(target, "[") {
		var err error
		host, portText, err = net.SplitHostPort(target)
		if err != nil {
			return portTarget{}, fmt.Errorf("invalid address %q: %w", raw, err)
		}
	} else if i := strings.LastIndexByte(target, ':'); i >= 0 {
		host = target[:i]
		portText = target[i+1:]
	}
	if strings.HasPrefix(portText, "u") {
		udp = true
		portText = strings.TrimPrefix(portText, "u")
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 0 || port > 65535 {
		return portTarget{}, fmt.Errorf("invalid port in %q", raw)
	}
	network := "tcp4"
	if udp {
		network = "udp4"
	}
	if strings.Contains(host, ":") {
		if udp {
			network = "udp6"
		} else {
			network = "tcp6"
		}
	}
	spec := strconv.Itoa(port)
	if host != "" {
		spec = net.JoinHostPort(host, strconv.Itoa(port))
	}
	if udp {
		spec = "u" + spec
	}
	return portTarget{host: host, port: port, network: network, spec: spec, fd: fd}, nil
}

func listenConfig(network string) net.ListenConfig {
	return net.ListenConfig{Control: func(_, _ string, conn syscall.RawConn) error {
		var controlErr error
		if err := conn.Control(func(fd uintptr) {
			controlErr = setSockOptReuseAddr(fd)
			if controlErr == nil && strings.HasSuffix(network, "6") {
				controlErr = setSockOptIPv6Only(fd)
			}
		}); err != nil {
			return err
		}
		return controlErr
	}}
}

func (s *Starter) Run() error {
	//nolint:errcheck
	defer s.Teardown()

	if s.pidFile != "" {
		f, err := acquirePIDFile(s.pidFile)
		if err != nil {
			return err
		}
		defer f.Close()
	}

	requestedFDs := make(map[int]struct{})
	for _, addr := range s.ports {
		target, err := parsePortTarget(addr)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to parse addr spec '%s': %s", addr, err)
			return err
		}
		if target.fd >= 0 {
			if target.fd < 3 {
				return fmt.Errorf("listener descriptor %d conflicts with standard streams", target.fd)
			}
			if _, ok := requestedFDs[target.fd]; ok {
				return fmt.Errorf("listener descriptor %d is specified more than once", target.fd)
			}
			requestedFDs[target.fd] = struct{}{}
		}

		var l net.Listener
		var pc net.PacketConn
		if strings.HasPrefix(target.network, "udp") {
			lc := listenConfig(target.network)
			pc, err = lc.ListenPacket(context.Background(), target.network, net.JoinHostPort(target.host, strconv.Itoa(target.port)))
		} else {
			lc := listenConfig(target.network)
			l, err = lc.Listen(context.Background(), target.network, net.JoinHostPort(target.host, strconv.Itoa(target.port)))
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to listen to %s:%s\n", target.spec, err)
			return err
		}
		s.mu.Lock()
		s.listeners = append(s.listeners, listener{listener: l, packet: pc, fd: target.fd, spec: target.spec})
		s.mu.Unlock()
	}

	for _, path := range s.paths {
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
		s.mu.Lock()
		s.listeners = append(s.listeners, listener{listener: l, spec: path})
		s.mu.Unlock()
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
	var restartTimer *time.Timer
	var restartC <-chan time.Time
	var autoRestartForced bool
	if autoRestartEnabled() {
		restartTimer = time.NewTimer(autoRestartInterval())
		restartC = restartTimer.C
	}
	defer func() {
		if restartTimer != nil {
			restartTimer.Stop()
		}
	}()

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
			_ = worker.Signal(sigToSend)
		}

		for len(oldWorkers) > 0 {
			st := <-workerCh
			fmt.Fprintf(os.Stderr, "worker %d died, status:%d\n", st.Pid(), grabExitStatus(st))
			delete(oldWorkers, st.Pid())
			if err := writeStatusFile(s.statusFile, statusMap(oldWorkers, 0, s.generation)); err != nil {
				fmt.Fprintf(os.Stderr, "failed to write status file: %s\n", err)
			}
		}
		fmt.Fprintf(os.Stderr, "exiting\n")
	}()

	for { // outer loop
		setEnv()

		// Just wait for the worker to exit, or for us to receive a signal
		for {
			// StartWorker can return nil when a signal arrives after a replacement
			// exits but before the next retry succeeds.
			currentPID := 0
			if p != nil {
				currentPID = p.Pid
			}
			if err := writeStatusFile(s.statusFile, statusMap(oldWorkers, currentPID, s.generation)); err != nil {
				fmt.Fprintf(os.Stderr, "failed to write status file: %s\n", err)
			}
			// restart == 2: respawn unconditionally
			// restart == 1: respawn only if no old workers are still alive
			// restart == 0: leave the worker alone
			//
			// Level 1 has no callers yet. It exists for ENABLE_AUTO_RESTART,
			// whose two thresholds in Server::Starter differ in exactly this
			// way: the ordinary interval waits for the old workers to go, the
			// forced one (twice the interval) does not.
			restart := 0

			select {
			case st := <-workerCh:
				// oops, the worker exited? check for its pid
				if p != nil && p.Pid == st.Pid() { // current worker
					exitSt := grabExitStatus(st)
					fmt.Fprintf(os.Stderr, "worker %d died unexpectedly with status %d, restarting\n", p.Pid, exitSt)
					setEnv()
					p = s.StartWorker(sigCh, workerCh)
					if restartTimer != nil {
						autoRestartForced = false
						restartTimer.Reset(autoRestartInterval())
					}
				} else {
					exitSt := grabExitStatus(st)
					fmt.Fprintf(os.Stderr, "old worker %d died, status:%d\n", st.Pid(), exitSt)
					delete(oldWorkers, st.Pid())
				}
			case sigReceived = <-sigCh:
				// Temporary fix
				switch sigReceived {
				case syscall.SIGHUP:
					// When we receive a HUP signal, we need to spawn a new worker.
					//
					// This is level 2, not 1, on purpose. Server::Starter runs its
					// HUP path unconditionally (Starter.pm's `if ($restart)`), while
					// level 1 below is gated on there being no live old workers.
					// Using level 1 here made a HUP a no-op whenever an earlier
					// worker was still shutting down, so repeated HUPs never
					// re-signalled it. See #9.
					fmt.Fprintf(os.Stderr, "received HUP (num_old_workers=TODO)\n")
					restart = 2
					sigToSend = s.signalOnHUP
				case syscall.SIGTERM:
					sigToSend = s.signalOnTERM
					return nil
				default:
					sigToSend = syscall.SIGTERM
					return nil
				}
			case <-restartC:
				if len(oldWorkers) == 0 {
					restart = 1
					autoRestartForced = false
				} else if autoRestartForced {
					restart = 2
					autoRestartForced = false
				} else {
					autoRestartForced = true
					if restartTimer != nil {
						restartTimer.Reset(autoRestartInterval())
					}
				}
			}

			if restart > 1 || restart > 0 && len(oldWorkers) == 0 {
				fmt.Fprintf(os.Stderr, "spawning a new worker (num_old_workers=TODO)\n")
				if p != nil {
					oldWorkers[p.Pid] = s.generation
				}
				setEnv()
				p = s.StartWorker(sigCh, workerCh)
				if restartTimer != nil {
					autoRestartForced = false
					restartTimer.Reset(autoRestartInterval())
				}
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
						_ = worker.Signal(s.signalOnHUP)
					}
				}
			}
		}
	}
}

func autoRestartEnabled() bool {
	value, ok := os.LookupEnv("ENABLE_AUTO_RESTART")
	if !ok {
		return false
	}
	enabled, _ := strconv.ParseBool(value)
	return enabled || value == "1"
}

func autoRestartInterval() time.Duration {
	interval := 360
	if value, ok := os.LookupEnv("AUTO_RESTART_INTERVAL"); ok {
		if parsed, err := strconv.Atoi(value); err == nil && parsed > 0 {
			interval = parsed
		}
	}
	return time.Duration(interval) * time.Second
}

func getKillOldDelay() time.Duration {
	autoRestart, _ := strconv.ParseBool(os.Getenv("ENABLE_AUTO_RESTART"))

	v, ok := os.LookupEnv("KILL_OLD_DELAY")
	if !ok {
		if autoRestart {
			return 5 * time.Second
		}
		return 0
	}

	// KILL_OLD_DELAY is set: honour it, including an explicit 0, even when
	// auto-restart is enabled. An unparseable value is treated as 0,
	// consistent with this function's existing tolerance for bad input.
	delay, _ := strconv.ParseInt(v, 10, 0)

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
		s.mu.RLock()
		descriptors := make([]int, len(s.listeners))
		used := make(map[int]struct{}, len(s.listeners))
		for i, l := range s.listeners {
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
		ports := make([]string, len(s.listeners))
		var err error
		for slot := range files {
			files[slot], err = os.OpenFile(os.DevNull, os.O_RDONLY, 0)
			if err != nil {
				s.mu.RUnlock()
				for _, file := range files {
					if file != nil {
						file.Close()
					}
				}
				panic(err)
			}
		}
		for i, l := range s.listeners {
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
				s.mu.RUnlock()
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
		s.mu.RUnlock()
		cmd.ExtraFiles = files

		s.generation++
		os.Setenv("SERVER_STARTER_PORT", strings.Join(ports, ";"))
		os.Setenv("SERVER_STARTER_GENERATION", fmt.Sprintf("%d", s.generation))

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
					go func(sig os.Signal) {
						sigCh <- sig
					}(sig)
					if sysSig, ok := sig.(syscall.Signal); ok {
						if sysSig != syscall.SIGHUP {
							gotSig = true
						}
					}
				}
			}

			// Check if we can find a process by its pid
			p := findWorker(pid)
			if gotSig || p != nil {
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

func (s *Starter) Teardown() error {
	if s.statusFile != "" {
		os.Remove(s.statusFile)
	}

	s.mu.RLock()
	for _, l := range s.listeners {
		if l.listener != nil {
			l.listener.Close()
		}
		if l.packet != nil {
			l.packet.Close()
		}
	}
	s.mu.RUnlock()

	return nil
}

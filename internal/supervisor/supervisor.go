package supervisor

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/lestrrat-go/server-starter/v2/internal/statefile"
)

// runState holds everything that is specific to a single Run invocation:
// the listeners it opened, the worker generation counter, the map of old
// (still-draining) worker pids, and the auto-restart timer. cfg points back
// at the immutable Starter configuration shared across runs. Because a
// runState is allocated fresh inside Run and never shared with any other
// goroutine, its fields need no lock: only the goroutine executing Run (and
// the synchronous calls it makes into startWorker/teardown) ever touch them.
type runState struct {
	cfg *Starter

	listeners  []listener
	generation int

	oldWorkers map[int]int

	restartTimer      *time.Timer
	restartC          <-chan time.Time
	autoRestartForced bool
}

func (s *Starter) stop() {
	p, _ := os.FindProcess(os.Getpid())
	_ = p.Signal(syscall.SIGTERM)
}

func (s *Starter) Run() error {
	rs := &runState{
		cfg:       s,
		listeners: make([]listener, 0, len(s.ports)+len(s.paths)),
	}
	defer rs.teardown()

	if s.pidFile != "" {
		f, err := statefile.Acquire(s.pidFile)
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
		rs.listeners = append(rs.listeners, listener{listener: l, packet: pc, fd: target.fd, spec: target.spec})
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
		rs.listeners = append(rs.listeners, listener{listener: l, spec: path})
	}

	rs.generation = 0
	os.Setenv("SERVER_STARTER_GENERATION", fmt.Sprintf("%d", rs.generation))

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
	p := rs.startWorker(sigCh, workerCh)
	rs.oldWorkers = make(map[int]int)
	var sigReceived os.Signal
	var sigToSend os.Signal
	if autoRestartEnabled() {
		rs.restartTimer = time.NewTimer(autoRestartInterval())
		rs.restartC = rs.restartTimer.C
	}
	defer func() {
		if rs.restartTimer != nil {
			rs.restartTimer.Stop()
		}
	}()

	defer func() {
		if p != nil {
			rs.oldWorkers[p.Pid] = rs.generation
		}

		fmt.Fprintf(os.Stderr, "received %s, sending %s to all workers:",
			signame(sigReceived),
			signame(sigToSend),
		)
		size := len(rs.oldWorkers)
		i := 0
		for pid := range rs.oldWorkers {
			i++
			fmt.Fprintf(os.Stderr, "%d", pid)
			if i < size {
				fmt.Fprintf(os.Stderr, ",")
			}
		}
		fmt.Fprintf(os.Stderr, "\n")

		for pid := range rs.oldWorkers {
			worker, err := os.FindProcess(pid)
			if err != nil {
				continue
			}
			_ = worker.Signal(sigToSend)
		}

		for len(rs.oldWorkers) > 0 {
			st := <-workerCh
			fmt.Fprintf(os.Stderr, "worker %d died, status:%d\n", st.Pid(), grabExitStatus(st))
			delete(rs.oldWorkers, st.Pid())
			if err := statefile.WriteStatus(s.statusFile, statefile.StatusMap(rs.oldWorkers, 0, rs.generation)); err != nil {
				fmt.Fprintf(os.Stderr, "failed to write status file: %s\n", err)
			}
		}
		fmt.Fprintf(os.Stderr, "exiting\n")
	}()

	setEnv()

	// Just wait for the worker to exit, or for us to receive a signal
	for {
		// startWorker can return nil when a signal arrives after a replacement
		// exits but before the next retry succeeds.
		currentPID := 0
		if p != nil {
			currentPID = p.Pid
		}
		if err := statefile.WriteStatus(s.statusFile, statefile.StatusMap(rs.oldWorkers, currentPID, rs.generation)); err != nil {
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
				p = rs.startWorker(sigCh, workerCh)
				if rs.restartTimer != nil {
					rs.autoRestartForced = false
					rs.restartTimer.Reset(autoRestartInterval())
				}
			} else {
				exitSt := grabExitStatus(st)
				fmt.Fprintf(os.Stderr, "old worker %d died, status:%d\n", st.Pid(), exitSt)
				delete(rs.oldWorkers, st.Pid())
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
		case <-rs.restartC:
			if len(rs.oldWorkers) == 0 {
				restart = 1
				rs.autoRestartForced = false
			} else if rs.autoRestartForced {
				restart = 2
				rs.autoRestartForced = false
			} else {
				rs.autoRestartForced = true
				if rs.restartTimer != nil {
					rs.restartTimer.Reset(autoRestartInterval())
				}
			}
		}

		if restart > 1 || restart > 0 && len(rs.oldWorkers) == 0 {
			fmt.Fprintf(os.Stderr, "spawning a new worker (num_old_workers=TODO)\n")
			if p != nil {
				rs.oldWorkers[p.Pid] = rs.generation
			}
			setEnv()
			p = rs.startWorker(sigCh, workerCh)
			if rs.restartTimer != nil {
				rs.autoRestartForced = false
				rs.restartTimer.Reset(autoRestartInterval())
			}
			fmt.Fprintf(os.Stderr, "new worker is now running, sending %s to old workers:", signame(sigToSend))
			size := len(rs.oldWorkers)
			if size == 0 {
				fmt.Fprintf(os.Stderr, "none\n")
			} else {
				i := 0
				for pid := range rs.oldWorkers {
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

				for pid := range rs.oldWorkers {
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

func (rs *runState) teardown() {
	if rs.cfg.statusFile != "" {
		os.Remove(rs.cfg.statusFile)
	}

	for _, l := range rs.listeners {
		if l.listener != nil {
			l.listener.Close()
		}
		if l.packet != nil {
			l.packet.Close()
		}
	}
}

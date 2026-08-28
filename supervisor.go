package starter

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
)

func (s *Starter) stop() {
	p, _ := os.FindProcess(os.Getpid())
	_ = p.Signal(syscall.SIGTERM)
}

func (s *Starter) Run() error {
	defer s.teardown()

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
	p := s.startWorker(sigCh, workerCh)
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
			// startWorker can return nil when a signal arrives after a replacement
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
					p = s.startWorker(sigCh, workerCh)
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
				p = s.startWorker(sigCh, workerCh)
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

func (s *Starter) teardown() {
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
}

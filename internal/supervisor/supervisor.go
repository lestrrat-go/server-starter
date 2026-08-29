package supervisor

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/lestrrat-go/server-starter/v2/internal/statefile"
)

const defaultShutdownGracePeriod = 5 * time.Second

// runState holds everything that is specific to a single Run invocation:
// the listeners it opened, the worker generation counter, the map of old
// (still-draining) worker pids, and the auto-restart timer. cfg points back
// at the immutable Starter configuration shared across runs. Because a
// runState is allocated fresh inside Run and never shared with any other
// goroutine, its fields need no lock: only the goroutine executing the loop
// (and the synchronous calls it makes into startWorker/teardown) ever touch
// them.
type runState struct {
	cfg *Starter

	pidFile     *statefile.PIDFile
	listeners   []listener
	descriptors []int

	generation int

	// envOverlay holds the most recently loaded envdir variables. It is
	// refreshed on every (re)load point that used to call setEnv(), and is
	// overlaid onto each spawned worker's environment in startWorker; it is
	// never applied to the supervisor's own process environment.
	envOverlay map[string]string

	oldWorkers map[int]int

	restartTimer *time.Timer
	restartC     <-chan time.Time
}

// Run acquires the pid file, binds every listener, performs the initial envdir
// load, and starts the supervisor lifecycle in a background goroutine. Worker
// command-start errors are retried when transient. Terminal launch errors stop
// the lifecycle and are reported by the returned Controller.
//
// Cancelling ctx is the only way to stop the run; the returned Controller's
// Hangup method requests a graceful worker restart.
func (s *Starter) Run(ctx context.Context) (*Controller, error) {
	return s.run(ctx, false)
}

// RunWithStartupCheck behaves like Run but waits for the initial worker to pass
// its startup check. It returns an error if the worker cannot start or exits
// during that check, so daemon children can report an exact startup result to
// the waiting parent process.
func (s *Starter) RunWithStartupCheck(ctx context.Context) (*Controller, error) {
	return s.run(ctx, true)
}

func (s *Starter) run(ctx context.Context, waitForStartup bool) (*Controller, error) {
	rs := &runState{
		cfg:       s,
		listeners: make([]listener, 0, len(s.ports)+len(s.paths)),
	}
	targets := make([]portTarget, 0, len(s.ports))
	requestedDescriptors := make([]int, 0, len(s.ports)+len(s.paths))
	for _, addr := range s.ports {
		target, err := parsePortTarget(addr)
		if err != nil {
			fmt.Fprintf(s.stderr, "failed to parse addr spec '%s': %s", addr, err)
			return nil, err
		}
		targets = append(targets, target)
		requestedDescriptors = append(requestedDescriptors, target.fd)
	}
	for range s.paths {
		requestedDescriptors = append(requestedDescriptors, -1)
	}
	descriptors, err := assignListenerDescriptors(requestedDescriptors)
	if err != nil {
		return nil, err
	}
	rs.descriptors = descriptors
	// Apply the public wire-format validation before acquiring the pid file or
	// binding any listener. startWorker applies the same rule when it emits
	// SERVER_STARTER_PORT.
	if err := validateListenerWireFormat(targets, s.paths, descriptors); err != nil {
		return nil, err
	}

	// Setup can fail partway through (e.g. the second of three listeners
	// refuses to bind). Until the loop goroutine takes ownership, this
	// defer is responsible for releasing whatever was already acquired.
	ok := false
	defer func() {
		if ok {
			return
		}
		if rs.pidFile != nil {
			rs.pidFile.Close()
		}
		rs.teardown()
	}()

	if s.pidFile != "" {
		f, err := statefile.Acquire(s.pidFile)
		if err != nil {
			return nil, err
		}
		rs.pidFile = f
	}

	for _, target := range targets {
		var l net.Listener
		var pc net.PacketConn
		if strings.HasPrefix(target.network, "udp") {
			lc := listenConfig(target.network)
			pc, err = lc.ListenPacket(ctx, target.network, net.JoinHostPort(target.host, strconv.Itoa(target.port)))
		} else {
			lc := listenConfig(target.network)
			l, err = lc.Listen(ctx, target.network, net.JoinHostPort(target.host, strconv.Itoa(target.port)))
		}
		if err != nil {
			fmt.Fprintf(s.stderr, "failed to listen to %s:%s\n", target.spec, err)
			return nil, err
		}
		rs.listeners = append(rs.listeners, listener{
			listener: l,
			packet:   pc,
			network:  target.network,
			host:     target.host,
			port:     target.port,
		})
	}

	configuredSockets := configuredSocketPaths(s.paths)
	for _, path := range s.paths {
		var l net.Listener
		if err := removeExistingUnixSocketWithConfiguredPaths(path, configuredSockets); err != nil {
			fmt.Fprintf(s.stderr, "failed to prepare socket file:%s:%s\n", path, err)
			return nil, err
		}
		lc := listenConfig(unixNetwork)
		l, err := lc.Listen(ctx, unixNetwork, path)
		if err != nil {
			fmt.Fprintf(s.stderr, "failed to listen file:%s:%s\n", path, err)
			return nil, err
		}
		rs.listeners = append(rs.listeners, listener{listener: l, network: unixNetwork, path: path})
	}

	rs.generation = 0

	// Okay, ready to launch the program now... Nothing in-process reads
	// SERVER_STARTER_GENERATION, and the supervisor must not mutate its own
	// environment, so it is only ever set on the worker's cmd.Env in
	// startWorker.
	if err := rs.reloadEnvdir(); err != nil {
		return nil, err
	}

	ctrl := newController()
	var startup chan error
	if waitForStartup {
		startup = make(chan error, 1)
	}
	go rs.loop(ctx, ctrl, startup)
	ok = true
	if startup != nil {
		if err := <-startup; err != nil {
			<-ctrl.Done()
			return nil, err
		}
	}

	return ctrl, nil
}

func workerStartCanceled(ctx context.Context, err error) bool {
	ctxErr := ctx.Err()
	return ctxErr != nil && errors.Is(err, ctxErr)
}

func removeExistingUnixSocket(path string) error {
	return removeExistingUnixSocketWithConfiguredPaths(path, nil)
}

func removeExistingUnixSocketWithConfiguredPaths(path string, configuredPaths map[string]struct{}) error {
	if runtime.GOOS == "linux" &&
		(path == "" || strings.HasPrefix(path, "@") || strings.HasPrefix(path, "\x00")) {
		return nil
	}
	if !safeSocketQuarantineAvailable() {
		_, err := os.Lstat(path)
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("inspect unix socket path %q: %w", path, err)
		}
		return fmt.Errorf("prepare unix socket quarantine for %q: %w", path, errSafeSocketCleanupUnavailable)
	}
	return removeSocketWithConfiguredPaths(path, configuredPaths, socketCleanupHooks{})
}

type socketCleanupHooks struct {
	afterQuarantineMkdir        func(string)
	afterQuarantineOpenFailure  func(string)
	beforeMove                  func()
	beforeRetain                func(string)
	afterRetentionIdentityCheck func(string)
	beforeCleanup               func(string)
}

func removeSocketWithHooks(path string, hooks socketCleanupHooks) error {
	return removeSocketWithConfiguredPaths(path, nil, hooks)
}

func removeSocketWithConfiguredPaths(
	path string,
	configuredPaths map[string]struct{},
	hooks socketCleanupHooks,
) error {
	quarantine, err := newSocketQuarantine(path, configuredPaths, hooks)
	if err != nil {
		return fmt.Errorf("prepare unix socket quarantine for %q: %w", path, err)
	}
	defer quarantine.close()

	if hooks.beforeMove != nil {
		hooks.beforeMove()
	}
	if err := quarantine.moveIn(); err != nil {
		if errors.Is(err, errSocketSourceUnavailable) || os.IsNotExist(err) {
			return finishSocketQuarantine(quarantine, nil)
		}
		return finishSocketQuarantine(
			quarantine,
			fmt.Errorf("quarantine unix socket path %q: %w", path, err),
		)
	}

	isSocket, err := quarantine.entryIsSocket()
	if err != nil {
		if errors.Is(err, errSocketSourceChanged) {
			if restoreErr := quarantine.restore(); restoreErr != nil {
				return finishSocketQuarantine(quarantine, fmt.Errorf("unix socket path changed during preparation: restore entry: %w", restoreErr))
			}
			return finishSocketQuarantine(quarantine, fmt.Errorf("unix socket path changed during preparation and is not a socket"))
		}
		return finishSocketQuarantine(
			quarantine,
			fmt.Errorf("inspect quarantined unix socket %q: %w; entry retained at %q", path, err, quarantine.location()),
		)
	}
	if !isSocket {
		if err := quarantine.restore(); err != nil {
			return finishSocketQuarantine(
				quarantine,
				fmt.Errorf(
					"unix socket path %q is not a socket: restore entry: %w; entry retained at %q",
					path,
					err,
					quarantine.location(),
				),
			)
		}
		return finishSocketQuarantine(quarantine, fmt.Errorf("unix socket path %q is not a socket", path))
	}

	if hooks.beforeRetain != nil {
		hooks.beforeRetain(quarantine.location())
	}
	if err := quarantine.retainEntry(); err != nil {
		if hooks.beforeCleanup != nil {
			hooks.beforeCleanup(quarantine.location())
		}
		return finishSocketQuarantine(
			quarantine,
			fmt.Errorf("retain quarantined unix socket %q: %w", path, err),
		)
	}
	if hooks.beforeCleanup != nil {
		hooks.beforeCleanup(quarantine.location())
	}
	return finishSocketQuarantine(quarantine, nil)
}

func finishSocketQuarantine(quarantine socketQuarantine, operationErr error) error {
	cleanupErr := quarantine.cleanup()
	if cleanupErr == nil {
		return operationErr
	}
	cleanupErr = fmt.Errorf("clean unix socket quarantine %q: %w", quarantine.location(), cleanupErr)
	return errors.Join(operationErr, cleanupErr)
}

// loop runs the supervisor's main lifecycle: it starts the initial worker,
// then waits for the worker to exit, for a hangup request (graceful
// restart), for the auto-restart timer, or for ctx to be cancelled. It owns
// teardown of everything Run acquired: listeners, the pid file, and the
// controller's done channel are all released here, not in Run, so they stay
// alive for as long as the loop is actually running.
func (rs *runState) loop(ctx context.Context, ctrl *Controller, startup chan<- error) {
	defer close(ctrl.done)
	defer rs.teardown()
	if rs.pidFile != nil {
		defer rs.pidFile.Close()
	}

	workerCh := make(chan processState)
	workerStateDone := make(chan struct{})
	p, err := rs.startWorker(ctx, workerCh, workerStateDone, startup != nil)
	if startup != nil {
		startupErr := err
		if startupErr == nil {
			startupErr = ctx.Err()
		}
		startup <- startupErr
	}
	if err != nil {
		if workerStartCanceled(ctx, err) {
			ctrl.setErr(ErrServerClosed)
			return
		}
		fmt.Fprintf(rs.cfg.stderr, "%s\n", err)
		ctrl.setErr(err)
		return
	}
	if p == nil {
		if ctx.Err() != nil {
			ctrl.setErr(ErrServerClosed)
		}
		return
	}
	rs.oldWorkers = make(map[int]int)
	var sigToSend os.Signal
	if rs.cfg.enableAutoRestart {
		rs.restartTimer = time.NewTimer(rs.cfg.autoRestartInterval)
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
		rs.shutdownWorkers(sigToSend, workerCh)
		close(workerStateDone)
	}()

	hangupPending := false
	// Just wait for the worker to exit, for a restart request, or for ctx
	// to be cancelled.
	for {
		// startWorker can return nil when cancellation arrives while a newly
		// spawned process is undergoing its liveness check.
		currentPID := 0
		if p != nil {
			currentPID = p.Pid
		}
		if err := statefile.WriteStatus(rs.cfg.statusFile, statefile.StatusMap(rs.oldWorkers, currentPID, rs.generation)); err != nil {
			fmt.Fprintf(rs.cfg.stderr, "failed to write status file: %s\n", err)
		}
		restart := false

		select {
		case st := <-workerCh:
			// oops, the worker exited? check for its pid
			if p != nil && p.Pid == st.Pid() { // current worker
				exitSt := grabExitStatus(rs.cfg.stderr, st)
				pid := p.Pid
				p = nil
				fmt.Fprintf(rs.cfg.stderr, "worker %d died unexpectedly with status %d, restarting\n", pid, exitSt)
				if err := rs.reloadEnvdir(); err != nil {
					sigToSend = rs.cfg.signalOnTERM
					ctrl.setErr(err)
					return
				}
				newWorker, err := rs.startWorker(ctx, workerCh, workerStateDone, false)
				if err != nil {
					sigToSend = rs.cfg.signalOnTERM
					if workerStartCanceled(ctx, err) {
						ctrl.setErr(ErrServerClosed)
						return
					}
					fmt.Fprintf(rs.cfg.stderr, "%s\n", err)
					ctrl.setErr(err)
					return
				}
				p = newWorker
				// A HUP received while startWorker was bringing up this
				// replacement belongs to the restart already in progress.
				select {
				case <-ctrl.hangup:
				default:
				}
				if rs.restartTimer != nil {
					rs.restartTimer.Reset(rs.cfg.autoRestartInterval)
				}
			} else {
				exitSt := grabExitStatus(rs.cfg.stderr, st)
				fmt.Fprintf(rs.cfg.stderr, "old worker %d died, status:%d\n", st.Pid(), exitSt)
				delete(rs.oldWorkers, st.Pid())
				if len(rs.oldWorkers) == 0 && hangupPending {
					// A HUP can arrive after this worker-exit case wins the
					// select but before the pending restart is consumed. Fold
					// that buffered request into the same completed drain.
					select {
					case <-ctrl.hangup:
					default:
					}
					hangupPending = false
					restart = true
					sigToSend = rs.cfg.signalOnHUP
				}
			}
		case <-ctx.Done():
			sigToSend = rs.cfg.signalOnTERM
			ctrl.setErr(ErrServerClosed)
			return
		case <-ctrl.hangup:
			fmt.Fprintf(rs.cfg.stderr, "received hangup request (num_old_workers=%d)\n", len(rs.oldWorkers))
			if len(rs.oldWorkers) == 0 {
				restart = true
				sigToSend = rs.cfg.signalOnHUP
			} else {
				hangupPending = true
				fmt.Fprintf(rs.cfg.stderr, "coalescing hangup request until old workers exit\n")
			}
		case <-rs.restartC:
			if len(rs.oldWorkers) == 0 {
				restart = true
			} else {
				if rs.restartTimer != nil {
					rs.restartTimer.Reset(rs.cfg.autoRestartInterval)
				}
			}
		}

		if restart {
			fmt.Fprintf(rs.cfg.stderr, "spawning a new worker (num_old_workers=%d)\n", len(rs.oldWorkers))
			if p != nil {
				rs.oldWorkers[p.Pid] = rs.generation
			}
			p = nil
			if err := rs.reloadEnvdir(); err != nil {
				sigToSend = rs.cfg.signalOnTERM
				ctrl.setErr(err)
				return
			}
			newWorker, err := rs.startWorker(ctx, workerCh, workerStateDone, false)
			if err != nil {
				sigToSend = rs.cfg.signalOnTERM
				if workerStartCanceled(ctx, err) {
					ctrl.setErr(ErrServerClosed)
					return
				}
				fmt.Fprintf(rs.cfg.stderr, "%s\n", err)
				ctrl.setErr(err)
				return
			}
			p = newWorker
			if rs.restartTimer != nil {
				rs.restartTimer.Reset(rs.cfg.autoRestartInterval)
			}
			fmt.Fprintf(rs.cfg.stderr, "new worker is now running, sending %s to old workers:", signame(sigToSend))
			size := len(rs.oldWorkers)
			if size == 0 {
				fmt.Fprintf(rs.cfg.stderr, "none\n")
			} else {
				i := 0
				for pid := range rs.oldWorkers {
					i++
					fmt.Fprintf(rs.cfg.stderr, "%d", pid)
					if i < size {
						fmt.Fprintf(rs.cfg.stderr, ",")
					}
				}
				fmt.Fprintf(rs.cfg.stderr, "\n")

				killOldDelay := rs.cfg.killOldDelay
				fmt.Fprintf(rs.cfg.stderr, "sleep %d secs\n", int(killOldDelay/time.Second))
				if killOldDelay > 0 {
					timer := time.NewTimer(killOldDelay)
					select {
					case <-timer.C:
					case <-ctx.Done():
						if !timer.Stop() {
							<-timer.C
						}
					}
				}

				fmt.Fprintf(rs.cfg.stderr, "killing old workers\n")

				for pid := range rs.oldWorkers {
					worker, err := os.FindProcess(pid)
					if err != nil {
						continue
					}
					_ = worker.Signal(rs.cfg.signalOnHUP)
				}
			}

			// A HUP that arrived after another restart source won the select
			// belongs to the restart that just completed. Consume it before the
			// next loop iteration can mistake it for a request for another
			// generation.
			select {
			case <-ctrl.hangup:
			default:
			}
		}
	}
}

// shutdownWorkers first gives every worker a bounded period to exit after
// the configured graceful signal, then force-stops survivors and waits for
// their exit status for one more bounded period. Returning after the second
// deadline lets the loop release its listeners and pid file even if the OS
// cannot reap a worker promptly.
func (rs *runState) shutdownWorkers(sig os.Signal, workerCh <-chan processState) {
	fmt.Fprintf(rs.cfg.stderr, "sending %s to all workers:", signame(sig))
	rs.printWorkerPIDs()

	for pid := range rs.oldWorkers {
		worker, err := os.FindProcess(pid)
		if err != nil {
			continue
		}
		if err := worker.Signal(sig); err != nil {
			fmt.Fprintf(rs.cfg.stderr, "failed to signal worker %d: %s\n", pid, err)
		}
	}

	if rs.waitForWorkers(workerCh, rs.cfg.shutdownGracePeriod) {
		fmt.Fprintf(rs.cfg.stderr, "exiting\n")
		return
	}

	fmt.Fprintf(rs.cfg.stderr, "forcing remaining workers to exit:")
	rs.printWorkerPIDs()
	for pid := range rs.oldWorkers {
		worker, err := os.FindProcess(pid)
		if err != nil {
			continue
		}
		if err := worker.Kill(); err != nil {
			fmt.Fprintf(rs.cfg.stderr, "failed to force worker %d to exit: %s\n", pid, err)
		}
	}

	if !rs.waitForWorkers(workerCh, rs.cfg.shutdownGracePeriod) {
		fmt.Fprintf(rs.cfg.stderr, "timed out waiting to reap workers:")
		rs.printWorkerPIDs()
	}
	fmt.Fprintf(rs.cfg.stderr, "exiting\n")
}

func (rs *runState) waitForWorkers(workerCh <-chan processState, timeout time.Duration) bool {
	if len(rs.oldWorkers) == 0 {
		return true
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for len(rs.oldWorkers) > 0 {
		select {
		case st := <-workerCh:
			fmt.Fprintf(rs.cfg.stderr, "worker %d died, status:%d\n", st.Pid(), grabExitStatus(rs.cfg.stderr, st))
			delete(rs.oldWorkers, st.Pid())
			if err := statefile.WriteStatus(
				rs.cfg.statusFile,
				statefile.StatusMap(rs.oldWorkers, 0, rs.generation),
			); err != nil {
				fmt.Fprintf(rs.cfg.stderr, "failed to write status file: %s\n", err)
			}
		case <-timer.C:
			return false
		}
	}
	return true
}

func (rs *runState) printWorkerPIDs() {
	if len(rs.oldWorkers) == 0 {
		fmt.Fprintf(rs.cfg.stderr, "none\n")
		return
	}

	i := 0
	for pid := range rs.oldWorkers {
		i++
		fmt.Fprintf(rs.cfg.stderr, "%d", pid)
		if i < len(rs.oldWorkers) {
			fmt.Fprintf(rs.cfg.stderr, ",")
		}
	}
	fmt.Fprintf(rs.cfg.stderr, "\n")
}

func (rs *runState) reloadEnvdir() error {
	overlay, err := reloadEnv(rs.cfg.envdir)
	if err != nil {
		return err
	}
	rs.envOverlay = overlay
	return nil
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

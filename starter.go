package starter

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	envload "github.com/lestrrat/go-envload"
	"github.com/pkg/errors"
)

func New(command string, options ...Option) *Starter {
	return &Starter{
		command: command,
		options: options, // This is stored as-is on purpose.
	}
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

func parsePortSpec(addr string) (string, int, error) {
	i := strings.IndexByte(addr, ':')
	portPart := ""
	if i < 0 {
		portPart = addr
		addr = ""
	} else {
		portPart = addr[i+1:]
		addr = addr[:i]
	}

	port, err := strconv.ParseInt(portPart, 10, 64)
	if err != nil {
		return "", -1, err
	}

	return addr, int(port), nil
}

// This keeps listening to INT,TERM,HUP, and ALRM signals,
// and queues them up into a destination channel
func acceptSignals(ctx context.Context, dst chan os.Signal) {
	src := make(chan os.Signal, 32) // up to 32 signals
	signal.Notify(src, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGALRM)
	signal.Ignore(syscall.SIGPIPE)
	defer close(dst)
	for {
		select {
		case <-ctx.Done():
			return
		case sig, ok := <-src:
			if !ok {
				return
			}
			dst <- sig
		}
	}
}

func wait(ctx context.Context, sigCh chan os.Signal, workerDone chan *exec.Cmd) *exec.Cmd {
	// Original code in lib/Server/Starter.pm (_wait3) looks... interesting
	// currently going to punt it in light of just "wait for a process to
	// finish or a signal is received"
	t := time.NewTicker(time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			p, _ := os.FindProcess(os.Getpid())
			p.Signal(os.Signal(syscall.SIGTERM))
			return nil
		case <-t.C:
			if len(sigCh) > 0 {
				return nil
			}
		case cmd := <-workerDone:
			return cmd
		}
	}
	return nil
}

var registerCleanupKey struct{}

func registerCleanup(ctx context.Context, f func()) {
	register, ok := ctx.Value(registerCleanupKey).(func(func()))
	if !ok {
		return
	}
	register(f)
}

func cleanup(ctx context.Context, ch chan func()) {
	var finalizers []func()
	for loop := true; loop; {
		select {
		case <-ctx.Done():
			loop = false
			continue
		case f, ok := <-ch:
			if ok {
				finalizers = append(finalizers, f)
			}
		}
	}
	for _, f := range finalizers {
		f()
	}
}

func (s *Starter) Run(ctx context.Context) error {
	var cancel func()
	ctx, cancel = context.WithCancel(ctx)
	defer cancel()

	var cmdArgs []string
	var dir string
	var interval time.Duration = time.Second
	var paths []string
	var pidFile string
	var listeners []listener
	var ports []string
	var sigonhup os.Signal = os.Signal(syscall.SIGTERM)
	var sigonterm os.Signal = os.Signal(syscall.SIGTERM)
	var statusFile string
	var noticeOutput io.Writer = os.Stderr
	var logStdout io.Writer = os.Stdout
	var logStderr io.Writer = os.Stderr
	var killOldDelay time.Duration = 5 * time.Second

	for _, opt := range s.options {
		switch opt.Name() {
		case "args":
			cmdArgs = opt.Value().([]string)
		case "auto_restart_interval":
			v := opt.Value().(int)
			os.Setenv(`AUTO_RESTART_INTERVAL`, strconv.Itoa(v))
		case "dir":
			dir = opt.Value().(string)
		case "enable_auto_restart":
			b := opt.Value().(bool)
			if b {
				os.Setenv(`ENABLE_AUTO_RESTART`, `1`)
			} else {
				os.Setenv(`ENABLE_AUTO_RESTART`, `0`)
			}
		case "envdir":
			os.Setenv("ENVDIR", opt.Value().(string))
		case "interval":
			interval = opt.Value().(time.Duration)
		case "kill_old_delay":
			killOldDelay = opt.Value().(time.Duration)
		case "paths":
			paths = opt.Value().([]string)
		case "pid_file":
			pidFile = opt.Value().(string)
		case "ports":
			ports = opt.Value().([]string)
		case "signal_on_hup":
			sigonhup = opt.Value().(os.Signal)
		case "signal_on_term":
			sigonterm = opt.Value().(os.Signal)
		case "status_file":
			statusFile = opt.Value().(string)
		case "notice_output":
			noticeOutput = opt.Value().(io.Writer)
		case "log_stdout":
			logStdout = opt.Value().(io.Writer)
		case "log_stderr":
			logStderr = opt.Value().(io.Writer)
		}
	}

	if envAsBool(`ENABLE_AUTO_RESTART`) {
		os.Setenv(`KILL_OLD_DELAY`, strconv.Itoa(int(killOldDelay/time.Second)))
	}

	notice := func(f string, args ...interface{}) {
		var buf bytes.Buffer
		fmt.Fprintf(&buf, f, args...)
		if buf.Len() == 0 {
			return
		}

		b := buf.Bytes()
		if b[len(b)-1] != '\n' {
			buf.WriteByte('\n')
		}
		buf.WriteTo(noticeOutput)
	}

	generation := 0 // This is SERVER_STARTER_GENERATION
	os.Setenv(`SERVER_STARTER_GENERATION`, `0`)

	cleanupCh := make(chan func())
	ctx = context.WithValue(ctx, registerCleanupKey, func(f func()) {
		cleanupCh <- f
	})
	go cleanup(ctx, cleanupCh)

	// start listening
	extraFiles := make([]*os.File, 0, len(ports)+len(paths))
	portSpecs := make([]string, 0, len(ports)+len(paths))
	for _, addr := range ports {
		var l net.Listener

		host, port, err := parsePortSpec(addr)
		if err != nil {
			notice("failed to parse addr spec '%s': %s", addr, err)
			return err
		}

		hostport := fmt.Sprintf("%s:%d", host, port)
		l, err = net.Listen("tcp4", hostport)
		if err != nil {
			notice("failed to listen to %s:%s\n", hostport, err)
			return err
		}

		spec := ""
		if host == "" {
			spec = fmt.Sprintf("%d", port)
		} else {
			spec = fmt.Sprintf("%s:%d", host, port)
		}
		f, err := l.(*net.TCPListener).File()
		if err != nil {
			return errors.Wrap(err, "failed to get fd from listener")
		}
		registerCleanup(ctx, func() { f.Close() })
		extraFiles = append(extraFiles, f)
		portSpecs = append(portSpecs, fmt.Sprintf("%s=%d", spec, len(portSpecs)+3))
		listeners = append(listeners, listener{listener: l, spec: spec})
	}

	for _, path := range paths {
		var l net.Listener
		if fl, err := os.Lstat(path); err == nil && fl.Mode()&os.ModeSocket == os.ModeSocket {
			notice("removing existing socket file:%s\n", path)
			err = os.Remove(path)
			if err != nil {
				notice("failed to remove existing socket file:%s:%s\n", path, err)
				return err
			}
		}
		_ = os.Remove(path)
		l, err := net.Listen("unix", path)
		if err != nil {
			notice("failed to listen file:%s:%s\n", path, err)
			return err
		}
		f, err := l.(*net.UnixListener).File()
		if err != nil {
			return errors.Wrap(err, "failed to get fd from listener")
		}
		registerCleanup(ctx, func() { f.Close() })
		extraFiles = append(extraFiles, f)
		portSpecs = append(portSpecs, fmt.Sprintf("%s=%d", path, len(portSpecs)+3))
		listeners = append(listeners, listener{listener: l, spec: path})
	}

	os.Setenv("SERVER_STARTER_PORT", strings.Join(portSpecs, ";"))

	// Note: environment variables that are set after this
	// will NOT be re-populated
	envLoader := envload.New()

	var statusFileCreated bool
	defer func() {
		if statusFileCreated {
			os.Remove(statusFile)
		}
	}()
	var currentWorker int // pid
	var lastRestartTime time.Time
	oldWorkers := map[int]int{} // pid to generation

	var updateStatus func() error
	switch fn := statusFile; fn {
	case "":
		updateStatus = func() error { return nil }
	default:
		updateStatus = func() error {
			tmpfn := fn + "." + strconv.Itoa(os.Getpid())
			f, err := os.OpenFile(tmpfn, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
			if err != nil {
				return errors.Wrapf(err, "failed to create temporary file:%s", fn)
			}
			statusFileCreated = true
			m := map[int]int{}
			for k, v := range oldWorkers {
				m[k] = v
			}
			if currentWorker > 0 {
				m[generation] = currentWorker
			}

			keys := make([]int, 0, len(oldWorkers)+1)
			for k := range oldWorkers {
				keys = append(keys, k)
			}
			sort.Ints(keys)
			for _, k := range keys {
				fmt.Fprintf(f, "%d:%d\n", k, m[k])
			}
			f.Close()
			return errors.Wrapf(os.Rename(tmpfn, fn), "failed to rename %s to %s", fn, tmpfn)
		}
	}

	// This watcher receives commands to watch for.
	workerSrc := make(chan *exec.Cmd)
	workerDone := make(chan *exec.Cmd)
	go monitor(ctx, workerSrc, workerDone)

	// signal handler here queues up signals to the other
	// channel, so that we can keep accepting signals while we
	// only really handle them once per loop
	sigCh := make(chan os.Signal, 32)
	go acceptSignals(ctx, sigCh)

	errTryExec := errors.New("keep trying")
	startCmd := func(cmd *exec.Cmd) error {
		if err := cmd.Start(); err != nil {
			notice("%s", err.Error())
			// We would LOVE to continue immediately, but we need to do the
			// same check-for-signals and etc here, so we go on..
		} else {
			notice("starting new worker %d", cmd.Process.Pid)
		}

		// Wait for up to `interval` seconds before
		// checking if this command (process) is alive
		time.Sleep(interval)

		// Check if we have received any signals while we were
		// waiting. this is a very dirty trick in that we are
		// mucking with a channel that is potentially being written
		// to concurrently :/
		nonhup := 0
		var bufferedSigs []os.Signal
		l := len(sigCh)
		for i := 0; i < l; i++ {
			s := <-sigCh
			bufferedSigs = append(bufferedSigs, s)
			if s != os.Signal(syscall.SIGHUP) {
				// do not immediately stop... read all
				nonhup++
			}
		}
		if len(bufferedSigs) > 0 {
			go func() {
				for _, s := range bufferedSigs {
					sigCh <- s
				}
			}()
			if nonhup > 0 { // bailout
				return errors.New("received signal while waiting")
			}
		}

		// Want to check if the given PID is still alive.
		// This is not a great way to do it b/c we're not
		// even sure the Pid we're looking for is the same
		// process as the one we spawned, but... this is
		// so far the best we can do
		// Note: Does this work on windows?
		if cmd.Process != nil {
			p, err := os.FindProcess(cmd.Process.Pid)
			if err == nil {
				if err := p.Signal(os.Signal(syscall.Signal(0))); err == nil {
					return nil
				}
			}
		}

		switch {
		case cmd.ProcessState != nil:
			notice("new worker %d seems to have failed to start, exit status:%d", cmd.ProcessState.Pid(), grabExitStatus(cmd.ProcessState))
		case cmd.Process != nil:
			notice("new worker %d seems to have failed to start", cmd.Process.Pid)
		default:
			notice("new worker seems to have failed to start")
		}
		return errTryExec
	}

	if pidFile != "" {
		f, err := os.OpenFile(pidFile, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
		if err != nil {
			return errors.Wrapf(err, "failed to open file:%s", pidFile)
		}
		defer f.Close()
		defer os.Remove(f.Name())

		if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
			return errors.Wrapf(err, "flock failed(%s)", pidFile)
		}
		fmt.Fprintf(f, "%d\n", os.Getpid())
		if err := f.Sync(); err != nil {
			return errors.Wrapf(err, "failed to sync file(%s)", pidFile)
		}
	}

	newCommand := func() *exec.Cmd {
		cmd := exec.Command(s.command, cmdArgs...)
		if dir != "" {
			cmd.Dir = dir
		}
		cmd.Stdout = logStdout
		cmd.Stderr = logStderr
		cmd.ExtraFiles = extraFiles
		return cmd
	}

	startWorker := func() error {
		for loop := true; loop; {
			generation++
			os.Setenv(`SERVER_STARTER_GENERATION`, strconv.Itoa(generation))

			cmd := newCommand()
			switch err := startCmd(cmd); err {
			case nil:
				loop = false
				currentWorker = cmd.Process.Pid
				lastRestartTime = time.Now()
				updateStatus()
				workerSrc <- cmd
			case errTryExec:
				// keep trying
			default:
				return errors.Wrap(err, "failed to start command")
			}
		}

		return nil
	}

	var cleanupWorkers = func(sig os.Signal) {
		termSig := os.Signal(syscall.SIGTERM)
		if sig == termSig {
			termSig = sigonterm
		}

		if currentWorker > 0 {
			oldWorkers[currentWorker] = envAsInt(`SERVER_STARTER_GENERATION`)
			currentWorker = 0
		}
		var buf bytes.Buffer
		fmt.Fprintf(&buf, "received %s, sending %s to all workers:", signame(sig), signame(termSig))
		keys := make([]int, 0, len(oldWorkers))
		for k := range oldWorkers {
			keys = append(keys, k)
		}
		sort.Ints(keys)
		for i, k := range keys {
			fmt.Fprintf(&buf, "%d", k)
			if i < len(keys)-1 {
				buf.WriteByte(',')
			}
		}
		notice(buf.String())

		for _, pid := range keys {
			p, err := os.FindProcess(pid)
			if err != nil { // XXX to be safe, let's delete this pid
				delete(oldWorkers, pid)
			}
			p.Signal(termSig)
		}

		for len(oldWorkers) > 0 {
			cmd, ok := <-workerDone
			if !ok {
				panic("workerDone channel closed while still waiting for children to be reaped")
			}
			notice("worker %d died, status:%d", cmd.ProcessState.Pid(), grabExitStatus(cmd.ProcessState))
			delete(oldWorkers, cmd.ProcessState.Pid())
			updateStatus()
		}
	}

	if err := startWorker(); err != nil {
		return errors.Wrap(err, "failed to start worker")
	}

	for {
		// wait for next signal (or when auto-restart becomes necessary)
		exited := wait(ctx, sigCh, workerDone)

		// reload env if necessary
		envLoader.Restore(envload.WithLoadEnvdir(true))

		if envAsBool(`ENABLE_AUTO_RESTART`) {
			if os.Getenv("AUTO_RESTART_INTERVAL") == "" {
				os.Setenv("AUTO_RESTART_INTERVAL", "360")
			}
		}

		if exited != nil { // got some command exit
			pid := exited.ProcessState.Pid()
			if pid == currentWorker {
				notice("worker %d died unexpectedly with status: %d, restarting\n", pid, grabExitStatus(exited.ProcessState))
				if err := startWorker(); err != nil {
					return errors.Wrap(err, "failed to start worker")
				}
			} else {
				notice("old worker %d died, status:%d", pid, grabExitStatus(exited.ProcessState))
				delete(oldWorkers, pid)
				updateStatus()
			}
		}

		var restart bool
		for loop := true; loop; {
			select {
			case sig := <-sigCh:
				switch sig {
				case syscall.SIGHUP:
					notice("received HUP, spawning a new worker")
					restart = true
					loop = false
				case syscall.SIGALRM:
					loop = false
				default:
					cleanupWorkers(sig)
					return nil
				}
			default:
				loop = false
			}
		}

		if !restart && envAsBool("ENABLE_AUTO_RESTART") {
			autoRestartInterval := envAsDuration("AUTO_RESTART_INTERVAL")
			elapsedSinceRestart := time.Since(lastRestartTime)
			if elapsedSinceRestart >= autoRestartInterval && len(oldWorkers) == 0 {
				notice("autorestart triggered (interval=%s)", autoRestartInterval)
				restart = true
			} else if elapsedSinceRestart >= autoRestartInterval*2 {
				notice("autorestart triggered (forced, interval=%s)", autoRestartInterval)
			}
		}

		if restart {
			oldWorkers[currentWorker] = generation
			if err := startWorker(); err != nil {
				return errors.Wrap(err, "failed to restart worker")
			}

			var buf bytes.Buffer
			l := len(oldWorkers)
			if l == 0 {
				buf.WriteString("none")
			} else {
				i := 0
				for pid := range oldWorkers {
					buf.WriteString(strconv.Itoa(pid))
					if i < l-1 {
						buf.WriteByte(',')
					}
				}
			}
			notice("new worker is now running, sending %s to old workers: %s", signame(sigonhup), buf.String())

			killOldDelay := envAsDuration(`KILL_OLD_DELAY`)
			if killOldDelay == 0 {
				if envAsBool(`ENABLE_AUTO_RESTART`) {
					killOldDelay = 5 * time.Second
				} else {
					killOldDelay = 1
				}
			}

			time.Sleep(killOldDelay)
			for pid := range oldWorkers {
				worker, err := os.FindProcess(pid)
				if err != nil {
					continue
				}
				worker.Signal(sigonhup)
			}
		}
	}
}

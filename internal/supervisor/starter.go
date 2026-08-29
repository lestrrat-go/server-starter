package supervisor

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

type Config interface {
	Args() []string
	Command() string
	Dir() string             // Directory to chdir to before executing the command
	Interval() time.Duration // Time between checks for liveness
	PidFile() string
	Ports() []string                  // Ports to bind to; address components cannot contain ";" or "="
	Paths() []string                  // UNIX socket paths; ";" and "=" are reserved wire delimiters
	SignalOnHUP() (os.Signal, error)  // Signal to send when HUP is received
	SignalOnTERM() (os.Signal, error) // Signal to send when TERM is received
	StatusFile() string
	Envdir() string                     // Directory of files to load into each worker's environment
	EnableAutoRestart() bool            // Whether to restart workers automatically on a timer
	AutoRestartInterval() time.Duration // Interval between automatic restarts
	KillOldDelay() time.Duration        // Delay before signalling old workers after a restart

	// Stdout and Stderr are where the worker's own output and the
	// supervisor's own diagnostics are written. A nil return from either
	// falls back to the process-level os.Stdout/os.Stderr.
	Stdout() io.Writer
	Stderr() io.Writer
}

// Starter holds validated, immutable configuration for a supervisor run. It
// carries no per-run state, so one Starter may be shared across goroutines
// and Run multiple times concurrently to spawn independent supervised runs.
// All per-invocation state lives in runState, allocated fresh inside Run.
type Starter struct {
	interval     time.Duration
	signalOnHUP  os.Signal
	signalOnTERM os.Signal

	// shutdownGracePeriod is internal policy rather than Config surface: all
	// callers get a bounded graceful shutdown without another required knob.
	// Tests shorten it on their own Starter instances.
	shutdownGracePeriod time.Duration
	// you can't set this in go:	backlog
	statusFile string
	pidFile    string
	dir        string
	ports      []string
	paths      []string
	command    string
	args       []string

	envdir              string
	enableAutoRestart   bool
	autoRestartInterval time.Duration
	killOldDelay        time.Duration

	stdout io.Writer
	stderr io.Writer
}

func hasPathSeparator(path string) bool {
	for i := range len(path) {
		if os.IsPathSeparator(path[i]) {
			return true
		}
	}
	return false
}

func commandForValidation(command, dir string) string {
	if command == "" || dir == "" || filepath.IsAbs(command) || os.IsPathSeparator(command[0]) {
		return command
	}

	volume := filepath.VolumeName(command)
	if volume != "" {
		if !strings.EqualFold(volume, filepath.VolumeName(dir)) {
			return command
		}
		return filepath.Join(dir, command[len(volume):])
	}

	if !hasPathSeparator(command) {
		return command
	}
	return dir + string(os.PathSeparator) + command
}

// NewStarter creates a new Starter object. Config parameter may NOT be
// nil, as `Ports` and/or `Paths`, and `Command` are required
func NewStarter(c Config) (*Starter, error) {
	if c == nil {
		return nil, fmt.Errorf("config argument must be non-nil")
	}

	var signalOnHUP os.Signal = syscall.SIGTERM
	var signalOnTERM os.Signal = syscall.SIGTERM
	signalOnHUPValue, err := c.SignalOnHUP()
	if err != nil {
		return nil, fmt.Errorf("signal on HUP: %w", err)
	}
	if signalOnHUPValue != nil {
		signalOnHUP = signalOnHUPValue
	}

	signalOnTERMValue, err := c.SignalOnTERM()
	if err != nil {
		return nil, fmt.Errorf("signal on TERM: %w", err)
	}
	if signalOnTERMValue != nil {
		signalOnTERM = signalOnTERMValue
	}

	command := c.Command()
	if command == "" {
		return nil, fmt.Errorf("argument Command must be specified")
	}
	dir := c.Dir()
	if _, err := exec.LookPath(commandForValidation(command, dir)); err != nil {
		return nil, err
	}

	// A Config that returns nil for either writer falls back to the
	// process-level stream, so every existing Config implementer keeps
	// working without having to grow an opinion about logging.
	stdout := c.Stdout()
	if stdout == nil {
		stdout = os.Stdout
	}
	stderr := c.Stderr()
	if stderr == nil {
		stderr = os.Stderr
	}

	s := &Starter{
		args:                c.Args(),
		command:             command,
		dir:                 dir,
		interval:            c.Interval(),
		pidFile:             c.PidFile(),
		ports:               c.Ports(),
		paths:               c.Paths(),
		signalOnHUP:         signalOnHUP,
		signalOnTERM:        signalOnTERM,
		shutdownGracePeriod: defaultShutdownGracePeriod,
		statusFile:          c.StatusFile(),
		envdir:              c.Envdir(),
		enableAutoRestart:   c.EnableAutoRestart(),
		autoRestartInterval: c.AutoRestartInterval(),
		killOldDelay:        c.KillOldDelay(),
		stdout:              stdout,
		stderr:              stderr,
	}

	return s, nil
}

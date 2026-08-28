package supervisor

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"
)

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

// Starter holds validated, immutable configuration for a supervisor run. It
// carries no per-run state, so one Starter may be shared across goroutines
// and Run multiple times concurrently to spawn independent supervised runs.
// All per-invocation state lives in runState, allocated fresh inside Run.
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
		pidFile:      c.PidFile(),
		ports:        c.Ports(),
		paths:        c.Paths(),
		signalOnHUP:  signalOnHUP,
		signalOnTERM: signalOnTERM,
		statusFile:   c.StatusFile(),
	}

	return s, nil
}

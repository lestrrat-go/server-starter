package starter

import (
	"io"
	"net"
	"os"
	"syscall"
	"time"

	"github.com/lestrrat/go-server-starter/internal/env"
)

const version = `0.0.2`

var successStatus syscall.WaitStatus
var failureStatus syscall.WaitStatus

type listener struct {
	listener net.Listener
	spec     string // path or port spec
}

type Option interface {
	Name() string
	Value() interface{}
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
	options      []Option
	interval     time.Duration
	envLoader    *env.Loader
	noticeWriter io.Writer
	extraFiles   []*os.File
	portSpecs    []string

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

type WorkerState int

const (
	WorkerStarted WorkerState = iota
	ErrFailedToStart
)

type CLI struct{}
type boolOpt struct {
	Valid bool
	Value bool
}
type intOpt struct {
	Valid bool
	Value int
}

type stringOpt struct {
	Valid bool
	Value string
}



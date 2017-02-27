package starter

import (
	"net"
	"syscall"
)

const version = `0.0.2`

var failureStatus syscall.WaitStatus

type listener struct {
	listener net.Listener
	spec     string // path or port spec
}

type Option interface {
	Name() string
	Value() interface{}
}

type Restarter struct {
	pidFile    string
	statusFile string
}

type Starter struct {
	options []Option
	command string
}

type processState interface {
	Pid() int
	Sys() interface{}
}

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

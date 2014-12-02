package main

import (
	"fmt"
	"os"
	"reflect"
	"time"

	"github.com/jessevdk/go-flags"
	"github.com/lestrrat/go-server-starter"
)

type options struct {
	OptArgs         []string
	OptCommand      string
	OptDir          string
	OptInterval     time.Duration // Time between checks for liveness
	OptPidFile      string
	OptPorts        []int     `long:"port" description:"TCP port to listen to (if omitted, will not bind to any ports)"`
	OptPaths        []string  // Paths (UNIX domain socket) to bind to
	OptSignalOnHUP  os.Signal // Signal to send when HUP is received
	OptSignalOnTERM os.Signal // Signal to send when TERM is received
	OptStatusFile   string
}

func (o options) Args() []string          { return o.OptArgs }
func (o options) Command() string         { return o.OptCommand }
func (o options) Dir() string             { return o.OptDir }
func (o options) Interval() time.Duration { return o.OptInterval }
func (o options) PidFile() string         { return o.OptPidFile }
func (o options) Ports() []int            { return o.OptPorts }
func (o options) Paths() []string         { return o.OptPaths }
func (o options) SignalOnHUP() os.Signal  { return o.OptSignalOnHUP }
func (o options) SignalOnTERM() os.Signal { return o.OptSignalOnTERM }
func (o options) StatusFile() string      { return o.OptStatusFile }

func showHelp() {
	// The ONLY reason we're not using go-flags' help option is
	// because I wanted to tweak the format just a bit... but
	// there wasn't an easy way to do so
	os.Stderr.WriteString(`
Usage: start_server [options] cmd [args...]

Options:
`)

	t := reflect.TypeOf(options{})
	for i := 0; i < t.NumField(); i++ {
		tag := t.Field(i).Tag
		if tag == "" {
			continue
		}
		var o string
		if s := tag.Get("short"); s != "" {
			o = fmt.Sprintf("-%s, --%s", tag.Get("short"), tag.Get("long"))
		} else {
			o = fmt.Sprintf("--%s", tag.Get("long"))
		}

		fmt.Fprintf(
			os.Stderr,
			"  %-21s %s\n",
			o,
			tag.Get("description"),
		)
	}
}

func main() {
	os.Exit(_main())
}

func _main() (st int) {
	st = 1

	opts := &options{}
	p := flags.NewParser(opts, flags.PrintErrors|flags.PassDoubleDash)
	args, err := p.Parse()
	if err != nil {
		showHelp()
		return
	}
	opts.OptCommand = args[0]
	if len(args) > 1 {
		opts.OptArgs = args[1:]
	}

	s, err := starter.NewStarter(opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		return
	}
	s.Run()
	st = 0
	return
}
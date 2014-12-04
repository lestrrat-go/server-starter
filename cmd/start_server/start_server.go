package main

import (
	"fmt"
	"os"
	"reflect"
	"strings"
	"time"

	"github.com/jessevdk/go-flags"
	"github.com/lestrrat/go-server-starter"
)

type options struct {
	OptArgs         []string
	OptCommand      string
	OptDir          string    `long:"dir" arg:"path" description:"working directory, start_server do chdir to before exec (optional)"`
	OptInterval     int       `long:"interval" arg:"seconds" description:"minimum interval (in seconds) to respawn the server program (default: 1)"`
	OptPidFile      string    `long:"pid-file" arg:"filename" description:"if set, writes the process id of the start_server process to the file"`
	OptPorts        []int     `long:"port" arg:"(port|host:port)" description:"TCP port to listen to (if omitted, will not bind to any ports)"`
	OptPaths        []string  `long:"path" arg:"path" description:"path at where to listen using unix socket (optional)"`
	OptSignalOnHUP  os.Signal `long:"signal-on-hup" arg:"Signal" description:"name of the signal to be sent to the server process when start_server\nreceives a SIGHUP (default: SIGTERM). If you use this option, be sure to\nalso use '--signal-on-term' below."`
	OptSignalOnTERM os.Signal `long:"signal-on-term" arg:"Signal" description:"name of the signal to be sent to the server process when start_server\nreceives a SIGTERM (default: SIGTERM)"`
	OptStatusFile   string
}

func (o options) Args() []string          { return o.OptArgs }
func (o options) Command() string         { return o.OptCommand }
func (o options) Dir() string             { return o.OptDir }
func (o options) Interval() time.Duration { return time.Duration(o.OptInterval) * time.Second }
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
Usage:
      start_server [options] -- server-prog server-arg1 server-arg2 ...

      # start Plack using Starlet listening at TCP port 8000
      start_server --port=8000 -- plackup -s Starlet --max-workers=100 index.psgi

Options:
`)

	t := reflect.TypeOf(options{})
	for i := 0; i < t.NumField(); i++ {
		tag := t.Field(i).Tag
		if tag == "" {
			continue
		}
		if s := tag.Get("long"); s != "" {
			fmt.Fprintf(os.Stderr, "  --%s", s)
			if a := tag.Get("arg"); a != "" {
				fmt.Fprintf(os.Stderr, "=%s", a)
			}
			fmt.Fprintf(os.Stderr, ":\n")
		}
		for _, l := range strings.Split(tag.Get("description"), "\n") {
			fmt.Fprintf(os.Stderr, "    %s\n", l)
		}
		fmt.Fprintf(os.Stderr, "\n")
	}
}

func main() {
	os.Exit(_main())
}

func _main() (st int) {
	st = 1

	opts := &options{OptInterval: -1}
	p := flags.NewParser(opts, flags.PrintErrors|flags.PassDoubleDash)
	args, err := p.Parse()
	if err != nil {
		showHelp()
		return
	}
	if opts.OptInterval < 0 {
		opts.OptInterval = 1
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
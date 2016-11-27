package starter

import (
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"time"

	flags "github.com/jessevdk/go-flags"
)

func NewCLI() *CLI {
	return &CLI{}
}

func makeOptionList(opts *options) []Option {
	var list []Option
	if len(opts.Args) > 0 {
		list = append(list, WithArgs(opts.Args))
	}
	if opts.AutoRestartInterval.Valid {
		list = append(list, WithAutoRestartInterval(time.Duration(opts.AutoRestartInterval.Value)*time.Second))
	}
	if opts.Dir != "" {
		list = append(list, WithDir(opts.Dir))
	}
	if opts.EnableAutoRestart.Valid {
		list = append(list, WithAutoRestart(opts.EnableAutoRestart.Value))
	}
	if opts.Envdir.Valid {
		list = append(list, WithEnvdir(opts.Envdir.Value))
	}
	if opts.Interval > -1 {
		list = append(list, WithInterval(time.Duration(opts.Interval) * time.Second))
	}
	if opts.KillOldDelay.Valid {
		list = append(list, WithKillOldDelay(time.Duration(opts.KillOldDelay.Value)*time.Second))
	}
	if len(opts.Paths) > 0 {
		list = append(list, WithPaths(opts.Paths))
	}
	if opts.PidFile != "" {
		list = append(list, WithPidFile(opts.PidFile))
	}
	if len(opts.Ports) > 0 {
		list = append(list, WithPorts(opts.Ports))
	}
	if opts.SignalOnHUP != "" {
		list = append(list, WithSignalOnHUP(SigFromName(opts.SignalOnHUP)))
	}
	if opts.SignalOnTERM != "" {
		list = append(list, WithSignalOnTERM(SigFromName(opts.SignalOnTERM)))
	}
	if opts.StatusFile != "" {
		list = append(list, WithStatusFile(opts.StatusFile))
	}
	return list
}
func (cli *CLI) Run(ctx context.Context) error {
	var opts options
	opts.Interval = -1 // allow 0
	p := flags.NewParser(&opts, flags.PrintErrors|flags.PassDoubleDash)
	args, err := p.Parse()
	if err != nil || opts.Help {
		showHelp()
		return nil
	}

	if opts.Version {
		fmt.Printf("%s\n", version)
		return nil
	}

	if opts.Interval <= 0 {
		opts.Interval = 1
	}

	if len(args) == 0 {
		return errors.New("server program not specified")
	}

	opts.Command = args[0]
	if len(args) > 1 {
		opts.Args = args[1:]
	}

	s := New(opts.Command, makeOptionList(&opts)...)
	return s.Run(ctx)
}

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

	// This weird indexing stuff is done purely to keep ourselves
	// compatible with the original start_server program
	// (This is the order that the help is displayed in)
	names := []string{
		"Ports",
		"Paths",
		"Dir",
		"Interval",
		"SignalOnHUP",
		"SignalOnTERM",
		"PidFile",
		"StatusFile",
		"Envdir",
		"EnableAutoRestart",
		"AutoRestartInterval",
		"KillOldDelay",
		"Restart",
		"Help",
		"Version",
	}

	for _, name := range names {
		f, ok := t.FieldByName(name)
		if !ok {
			continue
		}

		tag := f.Tag
		if tag == "" {
			continue
		}
		if s := tag.Get("long"); s != "" {
			fmt.Fprintf(os.Stderr, "  --%s", s)
			if a := tag.Get("arg"); a != "" {
				fmt.Fprintf(os.Stderr, "=%s", a)
			}
			if tag.Get("note") == "unimplemented" {
				fmt.Fprintf(os.Stderr, " (UNIMPLEMENTED)")
			}
			fmt.Fprintf(os.Stderr, ":\n")
		}
		for _, l := range strings.Split(tag.Get("description"), "\n") {
			fmt.Fprintf(os.Stderr, "    %s\n", l)
		}
		fmt.Fprintf(os.Stderr, "\n")
	}
}

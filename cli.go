package starter

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"strings"
	"time"

	"github.com/pkg/errors"
)

func NewCLI() *CLI {
	return &CLI{}
}

func makeOptionList(opts *options) []Option {
	var list []Option
	if len(opts.Args) > 0 {
		list = append(list, WithArgs(opts.Args...))
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
		list = append(list, WithInterval(time.Duration(opts.Interval)*time.Second))
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
		list = append(list, WithSignalOnHUP(sigFromName(opts.SignalOnHUP)))
	}
	if opts.SignalOnTERM != "" {
		list = append(list, WithSignalOnTERM(sigFromName(opts.SignalOnTERM)))
	}
	if opts.StatusFile != "" {
		list = append(list, WithStatusFile(opts.StatusFile))
	}
	return list
}

func (cli *CLI) ParseArgs(args ...string) (*options, error) {
	var opts options
	opts.Interval = -1 // allow 0
	if err := opts.Parse(args...); err != nil {
		return nil, errors.Wrap(err, "failed to parse arguments")
	}

	if opts.Interval < 0 {
		opts.Interval = 1
	}

	if len(opts.Args) == 0 {
		return nil, errors.New("server program not specified")
	}

	opts.Command = opts.Args[0]
	if len(opts.Args) > 1 {
		opts.Args = opts.Args[1:]
	} else {
		opts.Args = []string(nil)
	}

	return &opts, nil
}

func (cli *CLI) Run(ctx context.Context) error {
	opts, err := cli.ParseArgs(os.Args...)
	if err != nil {
		return err
	}

	if opts.Help {
		showHelp()
		return nil
	}

	if opts.Version {
		fmt.Printf("%s\n", version)
		return nil
	}

	if opts.Restart {
		if opts.PidFile == "" || opts.StatusFile == "" {
			return errors.New("--restart option requires --pid-file and --status-file to be set as well")
		}

		s := NewRestarter(opts.PidFile, opts.StatusFile)
		return s.Run(ctx)
	}

	s := New(opts.Command, makeOptionList(opts)...)
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

package main

import (
	"fmt"
	"os"
	"os/exec"
	"reflect"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/jessevdk/go-flags"
	starter "github.com/lestrrat-go/server-starter/v2"
)

const version = "0.0.2"

type options struct {
	OptArgs                []string
	OptCommand             string
	OptDir                 string   `long:"dir" arg:"path" description:"working directory, start_server do chdir to before exec (optional)"`
	OptInterval            int      `long:"interval" arg:"seconds" description:"minimum interval (in seconds) to respawn the server program (default: 1)"`
	OptPorts               []string `long:"port" arg:"(port|host:port|port=fd|host:port=fd)" description:"TCP or UDP port to listen to. Prefix the port with u for UDP; bracket IPv6 hosts."`
	OptPaths               []string `long:"path" arg:"path" description:"path at where to listen using unix socket (optional)"`
	OptSignalOnHUP         string   `long:"signal-on-hup" arg:"Signal" description:"name of the signal to be sent to the server process when start_server\nreceives a SIGHUP (default: TERM). If you use this option, be sure to\nalso use '--signal-on-term' below."`
	OptSignalOnTERM        string   `long:"signal-on-term" arg:"Signal" description:"name of the signal to be sent to the server process when start_server\nreceives a SIGTERM (default: TERM)"`
	OptPidFile             string   `long:"pid-file" arg:"filename" description:"if set, writes the process id of the start_server process to the file"`
	OptStatusFile          string   `long:"status-file" arg:"filename" description:"if set, writes the status of the server process(es) to the file"`
	OptLogFile             string   `long:"log-file" arg:"filename" description:"if set, appends standard output and standard error to the file"`
	OptDaemonize           bool     `long:"daemonize" description:"start the server in the background"`
	OptEnvdir              string   `long:"envdir" arg:"Envdir" description:"directory that contains environment variables to the server processes.\nIt is intended for use with \"envdir\" in \"daemontools\". This can be\noverwritten by environment variable \"ENVDIR\"."`
	OptEnableAutoRestart   bool     `long:"enable-auto-restart" description:"enables automatic restart by time. This can be overwritten by\nenvironment variable \"ENABLE_AUTO_RESTART\"."`
	OptAutoRestartInterval int      `long:"auto-restart-interval" arg:"seconds" description:"automatic restart interval (default 360). It is used with\n\"--enable-auto-restart\" option. This can be overwritten by environment\nvariable \"AUTO_RESTART_INTERVAL\"."`
	OptKillOldDelay        int      `long:"kill-old-delay" arg:"seconds" description:"time to suspend to send a signal to the old worker. The default value is\n5 when \"--enable-auto-restart\" is set, 0 otherwise. This can be\noverwritten by environment variable \"KILL_OLD_DELAY\"."`
	OptRestart             bool     `long:"restart" description:"this is a wrapper command that reads the pid of the start_server process\nfrom --pid-file, sends SIGHUP to the process and waits until the\nserver(s) of the older generation(s) die by monitoring the contents of\nthe --status-file"`
	OptStop                bool     `long:"stop" description:"reads the pid from --pid-file, sends SIGTERM, and waits for the process to exit"`
	OptHelp                bool     `long:"help" description:"prints this help"`
	OptVersion             bool     `long:"version" description:"prints the version number"`
}

func (o options) Args() []string          { return o.OptArgs }
func (o options) Command() string         { return o.OptCommand }
func (o options) Dir() string             { return o.OptDir }
func (o options) Interval() time.Duration { return time.Duration(o.OptInterval) * time.Second }
func (o options) PidFile() string         { return o.OptPidFile }
func (o options) Ports() []string         { return o.OptPorts }
func (o options) Paths() []string         { return o.OptPaths }
func (o options) SignalOnHUP() os.Signal  { return starter.SigFromName(o.OptSignalOnHUP) }
func (o options) SignalOnTERM() os.Signal { return starter.SigFromName(o.OptSignalOnTERM) }
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

	t := reflect.TypeFor[options]()

	// This weird indexing stuff is done purely to keep ourselves
	// compatible with the original start_server program
	// (This is the order that the help is displayed in)
	names := []string{
		"OptPorts",
		"OptPaths",
		"OptDir",
		"OptInterval",
		"OptSignalOnHUP",
		"OptSignalOnTERM",
		"OptPidFile",
		"OptStatusFile",
		"OptLogFile",
		"OptDaemonize",
		"OptEnvdir",
		"OptEnableAutoRestart",
		"OptAutoRestartInterval",
		"OptKillOldDelay",
		"OptRestart",
		"OptStop",
		"OptHelp",
		"OptVersion",
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

func main() {
	os.Exit(_main())
}

func _main() (st int) {
	st = 1

	opts := &options{OptInterval: -1}
	p := flags.NewParser(opts, flags.PrintErrors|flags.PassDoubleDash)
	args, err := p.Parse()
	if err != nil || opts.OptHelp {
		showHelp()
		return
	}

	if opts.OptVersion {
		fmt.Printf("%s\n", version)
		st = 0
		return
	}

	if opts.OptInterval < 0 {
		opts.OptInterval = 1
	}

	if opts.OptStop || opts.OptRestart {
		if opts.OptPidFile == "" {
			fmt.Fprintf(os.Stderr, "--pid-file is required with --stop or --restart\n")
			return
		}
		var err error
		if opts.OptStop {
			err = stopServer(opts.OptPidFile)
		} else {
			err = restartServer(opts.OptPidFile, opts.OptStatusFile)
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %s\n", err)
			return
		}
		return 0
	}

	if opts.OptDaemonize && os.Getenv("SERVER_STARTER_DAEMONIZED") != "1" {
		if err := daemonize(); err != nil {
			fmt.Fprintf(os.Stderr, "error: %s\n", err)
			return
		}
		return 0
	}

	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "server program not specified\n")
		return
	}

	opts.OptCommand = args[0]
	if len(args) > 1 {
		opts.OptArgs = args[1:]
	}

	if opts.OptEnvdir != "" {
		os.Setenv("ENVDIR", opts.OptEnvdir)
	}
	if opts.OptLogFile != "" {
		f, err := os.OpenFile(opts.OptLogFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %s\n", err)
			return
		}
		os.Stdout = f
		os.Stderr = f
	}

	// Export these into the environment the same way Perl's start_server
	// does (Starter.pm:50-55), and only when the flag was actually passed
	// so an unset flag does not clobber an inherited environment value.
	if p.FindOptionByLongName("kill-old-delay").IsSet() {
		os.Setenv("KILL_OLD_DELAY", strconv.Itoa(opts.OptKillOldDelay))
	}
	if p.FindOptionByLongName("enable-auto-restart").IsSet() {
		os.Setenv("ENABLE_AUTO_RESTART", "1")
	}
	if p.FindOptionByLongName("auto-restart-interval").IsSet() {
		os.Setenv("AUTO_RESTART_INTERVAL", strconv.Itoa(opts.OptAutoRestartInterval))
	}

	s, err := starter.NewStarter(opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		return
	}
	if err := s.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		return
	}
	st = 0
	return
}

func daemonize() error {
	cmd := exec.Command(os.Args[0], os.Args[1:]...)
	cmd.Env = append(os.Environ(), "SERVER_STARTER_DAEMONIZED=1")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Stdin = nil
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

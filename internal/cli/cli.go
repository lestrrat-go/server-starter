package cli

import (
	"fmt"
	"os"
	"strconv"

	"github.com/jessevdk/go-flags"
	starter "github.com/lestrrat-go/server-starter/v2"
	"github.com/lestrrat-go/server-starter/v2/internal/control"
)

// Run parses command-line arguments and dispatches to the appropriate
// start_server behavior (running the supervisor, --stop, --restart,
// --daemonize, --help, or --version). It returns the process exit code.
func Run() int {
	opts := &options{OptInterval: -1}
	p := flags.NewParser(opts, flags.PrintErrors|flags.PassDoubleDash)
	args, err := p.Parse()
	if err != nil || opts.OptHelp {
		showHelp()
		return 1
	}

	if opts.OptVersion {
		fmt.Fprintf(os.Stdout, "%s\n", version)
		return 0
	}

	if opts.OptInterval < 0 {
		opts.OptInterval = 1
	}

	if opts.OptStop || opts.OptRestart {
		if opts.OptPidFile == "" {
			fmt.Fprintf(os.Stderr, "--pid-file is required with --stop or --restart\n")
			return 1
		}
		var err error
		if opts.OptStop {
			err = control.Stop(opts.OptPidFile)
		} else {
			err = control.Restart(opts.OptPidFile, opts.OptStatusFile)
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %s\n", err)
			return 1
		}
		return 0
	}

	if opts.OptDaemonize && os.Getenv("SERVER_STARTER_DAEMONIZED") != "1" {
		if err := daemonize(); err != nil {
			fmt.Fprintf(os.Stderr, "error: %s\n", err)
			return 1
		}
		return 0
	}

	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "server program not specified\n")
		return 1
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
			return 1
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
		return 1
	}
	if err := s.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		return 1
	}
	return 0
}

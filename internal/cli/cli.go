package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jessevdk/go-flags"
	"github.com/lestrrat-go/server-starter/v2/internal/control"
	"github.com/lestrrat-go/server-starter/v2/internal/supervisor"
)

// controlTimeout bounds how long --stop and --restart wait for the target
// process to react. It preserves the 30-second default that used to be
// built into internal/control.
const controlTimeout = 30 * time.Second

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
	if err := opts.validateSignals(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		return 1
	}

	if opts.OptInterval < 0 {
		opts.OptInterval = 1
	}

	if opts.OptStop || opts.OptRestart {
		if opts.OptPidFile == "" {
			fmt.Fprintf(os.Stderr, "--pid-file is required with --stop or --restart\n")
			return 1
		}
		ctx, cancel := context.WithTimeout(context.Background(), controlTimeout)
		defer cancel()
		var err error
		if opts.OptStop {
			err = control.Stop(ctx, opts.OptPidFile)
		} else {
			err = control.Restart(ctx, opts.OptPidFile, opts.OptStatusFile)
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

	// stderr is where cli's own diagnostics below go. With --log-file it is
	// the opened file (so those messages land in the log, matching the
	// pre-existing behaviour of swapping the global os.Stderr); otherwise
	// it stays the process's real stderr.
	stderr := io.Writer(os.Stderr)
	if opts.OptLogFile != "" {
		f, err := openLogFile(opts.OptLogFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %s\n", err)
			return 1
		}
		opts.logWriter = f
		stderr = f
	}

	// Resolve envdir/auto-restart/kill-old-delay once, here, instead of
	// exporting them into the process environment for internal/supervisor
	// to read back (which is not safe: two supervisors running in one
	// process would race on the shared environment). Precedence is the
	// flag if it was explicitly passed, otherwise the ambient environment
	// variable, otherwise a default -- the same result the old exporting
	// code produced, without mutating the process environment to get it.
	opts.resolved = resolveSettings(opts, func(long string) bool {
		return p.FindOptionByLongName(long).IsSet()
	})

	s, err := supervisor.NewStarter(opts)
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}

	// internal/supervisor no longer touches os/signal: it is a library and
	// must not impose a signal policy on whatever embeds it. This is where
	// that policy lives for the start_server command itself.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ctrl, err := s.Run(ctx)
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	defer signal.Stop(sigCh)

	go func() {
		for sig := range sigCh {
			if sig == syscall.SIGHUP {
				ctrl.Hangup()
				continue
			}
			// INT, TERM, or QUIT: request shutdown and stop watching for
			// further signals, ctrl.Wait() below takes it from here.
			cancel()
			return
		}
	}()

	// A clean, ctx-driven shutdown is success, not failure: report only a
	// genuine runtime error.
	if err := ctrl.Wait(); err != nil && !errors.Is(err, supervisor.ErrServerClosed) {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	return 0
}

func openLogFile(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
}

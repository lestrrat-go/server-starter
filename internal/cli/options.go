package cli

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/lestrrat-go/server-starter/v2/internal/supervisor"
)

const version = "0.0.2"

type options struct {
	OptArgs                []string
	OptCommand             string
	OptDir                 string   `long:"dir" arg:"path" description:"working directory, start_server do chdir to before exec (optional)"`
	OptInterval            int      `long:"interval" arg:"seconds" description:"minimum interval (in seconds) to respawn the server program (default: 1)"`
	OptPorts               []string `long:"port" arg:"target[=fd]" description:"TCP or UDP port; explicit descriptors must be 3..1024 and may leave\nat most 256 unused slots (optional)"`
	OptPaths               []string `long:"path" arg:"path" description:"path at where to listen using unix socket (optional)"`
	OptSignalOnHUP         string   `long:"signal-on-hup" arg:"Signal" description:"name of the signal to be sent to the server process when start_server\nreceives a SIGHUP (default: TERM; unknown names are rejected). If you use\nthis option, be sure to also use '--signal-on-term' below."`
	OptSignalOnTERM        string   `long:"signal-on-term" arg:"Signal" description:"name of the signal to be sent to the server process when start_server\nreceives a SIGTERM (default: TERM; unknown names are rejected)"`
	OptPidFile             string   `long:"pid-file" arg:"filename" description:"if set, writes the process id of the start_server process to the file"`
	OptStatusFile          string   `long:"status-file" arg:"filename" description:"if set, writes the status of the server process(es) to the file"`
	OptLogFile             string   `long:"log-file" arg:"filename" description:"if set, appends standard output and standard error to the file"`
	OptDaemonize           bool     `long:"daemonize" description:"start the server in the background"`
	OptEnvdir              string   `long:"envdir" arg:"Envdir" description:"directory that contains environment variables to the server processes.\nIt is intended for use with \"envdir\" in \"daemontools\". This can be\noverwritten by environment variable \"ENVDIR\"."`
	OptEnableAutoRestart   bool     `long:"enable-auto-restart" description:"enables automatic restart by time. This can be overwritten by\nenvironment variable \"ENABLE_AUTO_RESTART\"."`
	OptAutoRestartInterval int      `long:"auto-restart-interval" arg:"seconds" description:"automatic restart interval (default 360). It is used with\n\"--enable-auto-restart\" option. This can be overwritten by environment\nvariable \"AUTO_RESTART_INTERVAL\"."`
	OptKillOldDelay        int      `long:"kill-old-delay" arg:"seconds" description:"time to suspend to send a signal to the old worker. The default value is\n5 when \"--enable-auto-restart\" is set, 0 otherwise. This can be\noverwritten by environment variable \"KILL_OLD_DELAY\"."`
	OptRestart             bool     `long:"restart" description:"this is a wrapper command that reads the pid of the start_server process\nfrom --pid-file, sends SIGHUP to the process and waits until the\nserver(s) of the older generation(s) die by monitoring the contents of\nthe --status-file. Cannot be used with --stop"`
	OptStop                bool     `long:"stop" description:"reads the pid from --pid-file, sends SIGTERM, and waits for the process to exit.\nCannot be used with --restart"`
	OptHelp                bool     `long:"help" description:"prints this help"`
	OptVersion             bool     `long:"version" description:"prints the version number"`

	// resolved carries the four settings whose precedence (flag if
	// explicitly passed, otherwise the ambient environment variable,
	// otherwise a default) is worked out once by resolveSettings, instead of
	// being re-derived on every Config method call. Run populates this after
	// parsing and before calling supervisor.NewStarter.
	resolved resolvedSettings

	// logWriter is the file opened for --log-file, if any. Run sets this
	// before calling supervisor.NewStarter; nil (the --log-file-unset case)
	// makes Stdout/Stderr return nil, and NewStarter substitutes the real
	// os.Stdout/os.Stderr for a nil Config writer.
	logWriter io.Writer
}

func (o options) Args() []string          { return o.OptArgs }
func (o options) Command() string         { return o.OptCommand }
func (o options) Dir() string             { return o.OptDir }
func (o options) Interval() time.Duration { return time.Duration(o.OptInterval) * time.Second }
func (o options) PidFile() string         { return o.OptPidFile }
func (o options) Ports() []string         { return o.OptPorts }
func (o options) Paths() []string         { return o.OptPaths }
func (o options) SignalOnHUP() os.Signal  { return supervisor.SigFromName(o.OptSignalOnHUP) }
func (o options) SignalOnTERM() os.Signal { return supervisor.SigFromName(o.OptSignalOnTERM) }
func (o options) StatusFile() string      { return o.OptStatusFile }

func (o options) Envdir() string                     { return o.resolved.envdir }
func (o options) EnableAutoRestart() bool            { return o.resolved.enableAutoRestart }
func (o options) AutoRestartInterval() time.Duration { return o.resolved.autoRestartInterval }
func (o options) KillOldDelay() time.Duration        { return o.resolved.killOldDelay }

func (o options) Stdout() io.Writer { return o.logWriter }
func (o options) Stderr() io.Writer { return o.logWriter }

func (o options) validateSignals() error {
	for _, setting := range []struct {
		option string
		value  string
	}{
		{option: "--signal-on-hup", value: o.OptSignalOnHUP},
		{option: "--signal-on-term", value: o.OptSignalOnTERM},
	} {
		if setting.value != "" && supervisor.SigFromName(setting.value) == nil {
			return fmt.Errorf("invalid %s value %q: unknown signal", setting.option, setting.value)
		}
	}
	return nil
}

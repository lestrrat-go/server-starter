package starter

import (
	"io"
	"os"
	"strconv"
	"time"
)

type valueOption struct {
	name  string
	value interface{}
}

func (o *valueOption) Name() string {
	return o.name
}

func (o *valueOption) Value() interface{} {
	return o.value
}

func WithAutoRestart(b bool) Option {
	return &valueOption{name: "enable_auto_restart", value: b}
}

func WithAutoRestartInterval(t time.Duration) Option {
	return &valueOption{name: "auto_restart_interval", value: t}
}

func WithArgs(a []string) Option {
	return &valueOption{name: "args", value: a}
}

func WithDir(dir string) Option {
	return &valueOption{name: "dir", value: dir}
}

func WithEnvdir(dir string) Option {
	return &valueOption{name: "envdir", value: dir}
}

func WithInterval(t time.Duration) Option {
	return &valueOption{name: "interval", value: t}
}

func WithKillOldDelay(t time.Duration) Option {
	return &valueOption{name: "kill_old_interval", value: t}
}

func WithPaths(l []string) Option {
	return &valueOption{name: "paths", value: l}
}

func WithPidFile(s string) Option {
	return &valueOption{name: "pid_file", value: s}
}

func WithPorts(l []string) Option {
	return &valueOption{name: "ports", value: l}
}

func WithSignalOnHUP(s os.Signal) Option {
	return &valueOption{name: "signal_on_hup", value: s}
}

func WithSignalOnTERM(s os.Signal) Option {
	return &valueOption{name: "signal_on_term", value: s}
}

func WithStatusFile(s string) Option {
	return &valueOption{name: "status_file", value: s}
}

func WithNoticeOutput(w io.Writer) Option {
	return &valueOption{name: "notice_output", value: w}
}

func (o *stringOpt) String() string {
	return o.Value
}

func (o *stringOpt) Set(s string) error {
	o.Valid = true
	o.Value = s
	return nil
}

func (o *intOpt) String() string {
	return strconv.FormatInt(int64(o.Value), 10)
}

func (o *intOpt) Set(s string) error {
	i, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return err
	}
	o.Valid = true
	o.Value = int(i)
	return nil
}

func (o *boolOpt) String() string {
	return strconv.FormatBool(o.Value)
}

func (o *boolOpt) Set(s string) error {
	b, err := strconv.ParseBool(s)
	if err != nil {
		return err
	}
	o.Valid = true
	o.Value = b
	return nil
}

type options struct {
	Args                []string
	AutoRestartInterval intOpt `long:"auto-restart-interval" arg:"seconds" description:"automatic restart interval (default 360). It is used with\n\"--enable-auto-restart\" option. This can be overwritten by environment\nvariable \"AUTO_RESTART_INTERVAL\"." note:"unimplemented"`
	Command             string
	Dir                 string    `long:"dir" arg:"path" description:"working directory, start_server do chdir to before exec (optional)"`
	EnableAutoRestart   boolOpt   `long:"enable-auto-restart" description:"enables automatic restart by time. This can be overwritten by\nenvironment variable \"ENABLE_AUTO_RESTART\"." note:"unimplemented"`
	Envdir              stringOpt `long:"envdir" arg:"Envdir" description:"directory that contains environment variables to the server processes.\nIt is intended for use with \"envdir\" in \"daemontools\". This can be\noverwritten by environment variable \"ENVDIR\"."`
	Interval            int       `long:"interval" arg:"seconds" description:"minimum interval (in seconds) to respawn the server program (default: 1)"`
	KillOldDelay        intOpt    `long:"kill-old-delay" arg:"seconds" description:"time to suspend to send a signal to the old worker. The default value is\n5 when \"--enable-auto-restart\" is set, 0 otherwise. This can be\noverwritten by environment variable \"KILL_OLD_DELAY\"."`
	Paths               []string  `long:"path" arg:"path" description:"path at where to listen using unix socket (optional)"`
	PidFile             string    `long:"pid-file" arg:"filename" description:"if set, writes the process id of the start_server process to the file"`
	Ports               []string  `long:"port" arg:"(port|host:port)" description:"TCP port to listen to (if omitted, will not bind to any ports)"`
	Restart             bool      `long:"restart" description:"this is a wrapper command that reads the pid of the start_server process\nfrom --pid-file, sends SIGHUP to the process and waits until the\nserver(s) of the older generation(s) die by monitoring the contents of\nthe --status-file" note:"unimplemented"`
	SignalOnHUP         string    `long:"signal-on-hup" arg:"Signal" description:"name of the signal to be sent to the server process when start_server\nreceives a SIGHUP (default: TERM). If you use this option, be sure to\nalso use '--signal-on-term' below."`
	SignalOnTERM        string    `long:"signal-on-term" arg:"Signal" description:"name of the signal to be sent to the server process when start_server\nreceives a SIGTERM (default: TERM)"`
	StatusFile          string    `long:"status-file" arg:"filename" description:"if set, writes the status of the server process(es) to the file"`
	Help                bool      `long:"help" description:"prints this help"`
	Version             bool      `long:"version" description:"printes the version number"`
}

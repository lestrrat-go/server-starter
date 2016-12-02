package starter

import (
	"io"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
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

func WithArgs(a ...string) Option {
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
	return &valueOption{name: "kill_old_delay", value: t}
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

func WithLogStdout(w io.Writer) Option {
	return &valueOption{name: "log_stdout", value: w}
}

func WithLogStderr(w io.Writer) Option {
	return &valueOption{name: "log_stderr", value: w}
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

type optsetter interface {
	Set(string) error
}

var osv = reflect.TypeOf((*optsetter)(nil)).Elem()

func (o *options) Parse(args ...string) error {
	rv := reflect.ValueOf(o).Elem()
	tv := rv.Type()
	names := map[string]reflect.Value{}
	for i := 0; i < tv.NumField(); i++ {
		f := tv.Field(i)
		if f.PkgPath != "" || f.Anonymous {
			continue
		}
		names[f.Tag.Get("long")] = rv.Field(i)
	}

	var arguments []string
	for len(args) > 0 {
		arg := args[0]
		args = args[1:]
		l := len(arg)
		if l == 2 && arg == "--" {
			// stop processing, everything after this is an argument
			if len(args) > 0 {
				arguments = append(arguments, args[1:]...)
			}
			args = []string(nil) // force loop termination
			break
		}

		if !strings.HasPrefix(arg, "--") {
			arguments = append(arguments, arg)
			continue
		}
		end := l
		var opval string
		if ei := strings.IndexByte(arg, '='); ei > -1 {
			end = ei
			if end < l-1 {
				opval = arg[end+1:]
			} else {
				return errors.Errorf("invalid argument '%s'", arg)
			}
		} else {
			// is the next argument the argument to this option
			if len(args) > 0 && !strings.HasPrefix(args[0], "--") {
				opval = args[0]
				args = args[1:]
			}
		}
		opname := arg[2:end]
		f := names[opname]
		opvalv := reflect.ValueOf(opval)
		switch f.Kind() {
		case reflect.Struct:
			if reflect.PtrTo(f.Type()).Implements(osv) {
				f.Addr().MethodByName("Set").Call([]reflect.Value{opvalv})
			} else if opvalv.Type().AssignableTo(f.Type()) {
				f.Set(opvalv)
			}
		case reflect.String:
			f.Set(opvalv)
		case reflect.Int:
			i, err := strconv.ParseInt(opval, 10, 64)
			if err != nil {
				return err
			}
			f.Set(reflect.ValueOf(int(i)))
		case reflect.Slice:
			f.Set(reflect.Append(f, opvalv))
		}
	}

	o.Args = arguments
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

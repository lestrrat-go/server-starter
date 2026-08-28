package cli

import (
	"fmt"
	"os"
	"reflect"
	"strings"
)

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

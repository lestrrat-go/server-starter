package starter

import (
	"fmt"
	"os"
	"strings"
	"syscall"
)

var niceSigNames map[syscall.Signal]string
var niceNameToSigs map[string]syscall.Signal

func makeNiceSigNames() map[syscall.Signal]string {
	m := map[syscall.Signal]string{
		syscall.SIGABRT: "ABRT",
		syscall.SIGALRM: "ALRM",
		syscall.SIGBUS:  "BUS",
		// syscall.SIGEMT:  "EMT",
		syscall.SIGFPE: "FPE",
		syscall.SIGHUP: "HUP",
		syscall.SIGILL: "ILL",
		// syscall.SIGINFO: "INFO",
		syscall.SIGINT: "INT",
		// syscall.SIGIOT:    "IOT",
		syscall.SIGKILL: "KILL",
		syscall.SIGPIPE: "PIPE",
		syscall.SIGQUIT: "QUIT",
		syscall.SIGSEGV: "SEGV",
		syscall.SIGTERM: "TERM",
		syscall.SIGTRAP: "TRAP",
	}

	// addPlatformDepdentNiceSigNames() is defined in the files
	// containing build tags
	return addPlatformDependentNiceSigNames(m)
}

func init() {
	niceSigNames = makeNiceSigNames()
	niceNameToSigs = make(map[string]syscall.Signal)
	for sig, name := range niceSigNames {
		niceNameToSigs[name] = sig
	}
}

func signame(s os.Signal) string {
	if ss, ok := s.(syscall.Signal); ok {
		return niceSigNames[ss]
	}
	return fmt.Sprintf("UNKNOWN (%s)", s)
}

func sigFromName(n string) os.Signal {
	n = strings.ToUpper(n)
	if strings.HasPrefix(n, "SIG") {
		n = n[3:] // remove SIG prefix
	}

	if sig, ok := niceNameToSigs[n]; ok {
		return sig
	}
	return nil
}

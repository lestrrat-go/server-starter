package supervisor

import (
	"fmt"
	"os"
	"strings"
	"syscall"
)

var niceSigNames map[syscall.Signal]string
var niceNameToSigs map[string]syscall.Signal

func makeNiceSigNamesCommon() map[syscall.Signal]string {
	return map[syscall.Signal]string{
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
}

func makeNiceSigNames() map[syscall.Signal]string {
	return addPlatformDependentNiceSigNames(makeNiceSigNamesCommon())
}

func init() {
	niceSigNames = makeNiceSigNames()
	niceNameToSigs = make(map[string]syscall.Signal)
	for sig, name := range niceSigNames {
		niceNameToSigs[name] = sig
	}
	if sig, ok := niceNameToSigs["XFSZ"]; ok {
		niceNameToSigs["GXFSZ"] = sig
	}
}

func signame(s os.Signal) string {
	if ss, ok := s.(syscall.Signal); ok {
		return niceSigNames[ss]
	}
	return "UNKNOWN"
}

// SigFromName returns the signal corresponding to the given signal name.
// An empty name means no signal was specified.
func SigFromName(n string) (os.Signal, error) {
	if n == "" {
		return nil, nil
	}

	name := strings.TrimPrefix(strings.ToUpper(n), "SIG")
	if sig, ok := niceNameToSigs[name]; ok {
		return sig, nil
	}
	return nil, fmt.Errorf("invalid signal name %q", n)
}

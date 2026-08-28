package cli

import (
	"os"
	"strconv"
	"time"
)

// defaultAutoRestartInterval mirrors internal/supervisor's historical
// default (previously baked into the AUTO_RESTART_INTERVAL reader).
const defaultAutoRestartInterval = 360 * time.Second

// defaultKillOldDelayWithAutoRestart is the delay used when nothing sets
// --kill-old-delay/KILL_OLD_DELAY and auto-restart is enabled.
const defaultKillOldDelayWithAutoRestart = 5 * time.Second

// resolvedSettings holds the four settings that used to be exported into
// the process environment (ENVDIR, ENABLE_AUTO_RESTART,
// AUTO_RESTART_INTERVAL, KILL_OLD_DELAY) purely so internal/supervisor
// could read them back. They are now resolved once, here, and carried on
// supervisor.Config instead.
type resolvedSettings struct {
	envdir              string
	enableAutoRestart   bool
	autoRestartInterval time.Duration
	killOldDelay        time.Duration
}

// flagIsSet reports whether the flag named by long was explicitly passed on
// the command line, as opposed to left at its zero value. It exists so
// resolveSettings can be exercised in tests without constructing a real
// go-flags parser.
type flagIsSet func(long string) bool

// resolveSettings applies the precedence rule shared by all four settings:
// the flag if it was explicitly set, otherwise the ambient environment
// variable, otherwise the default. This preserves the old code's
// behaviour (which exported into the environment only when a flag was
// passed, letting an unset flag fall through to whatever was already in
// the environment) without mutating the process environment to do it.
func resolveSettings(opts *options, isSet flagIsSet) resolvedSettings {
	var rs resolvedSettings

	rs.envdir = resolveEnvdir(isSet("envdir"), opts.OptEnvdir)
	rs.enableAutoRestart = resolveEnableAutoRestart(isSet("enable-auto-restart"), opts.OptEnableAutoRestart)
	rs.autoRestartInterval = resolveAutoRestartInterval(isSet("auto-restart-interval"), opts.OptAutoRestartInterval)
	rs.killOldDelay = resolveKillOldDelay(isSet("kill-old-delay"), opts.OptKillOldDelay, rs.enableAutoRestart)

	return rs
}

// resolveEnvdir: flag if set, otherwise the ambient ENVDIR, otherwise
// empty.
func resolveEnvdir(isSet bool, flagValue string) string {
	if isSet {
		return flagValue
	}
	return os.Getenv("ENVDIR")
}

// resolveEnableAutoRestart: flag if set, otherwise the ambient
// ENABLE_AUTO_RESTART parsed the same way internal/supervisor used to parse
// it (strconv.ParseBool, or the literal "1"), otherwise false.
func resolveEnableAutoRestart(isSet bool, flagValue bool) bool {
	if isSet {
		return flagValue
	}
	value, ok := os.LookupEnv("ENABLE_AUTO_RESTART")
	if !ok {
		return false
	}
	enabled, _ := strconv.ParseBool(value)
	return enabled || value == "1"
}

// resolveAutoRestartInterval matches internal/supervisor's old
// autoRestartInterval() exactly: a raw value that fails to parse, or is not
// positive, falls back to the 360-second default. This validation applied
// uniformly to flag and ambient values alike in the old code (both were
// funneled through AUTO_RESTART_INTERVAL and read back by the same
// function), so it is reproduced here for both sources.
func resolveAutoRestartInterval(isSet bool, flagSeconds int) time.Duration {
	var raw string
	var ok bool
	if isSet {
		raw, ok = strconv.Itoa(flagSeconds), true
	} else {
		raw, ok = os.LookupEnv("AUTO_RESTART_INTERVAL")
	}
	if ok {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			return time.Duration(parsed) * time.Second
		}
	}
	return defaultAutoRestartInterval
}

// resolveKillOldDelay matches internal/supervisor's old getKillOldDelay():
// when nothing sets the delay (neither the flag nor the ambient
// environment), the default is 5 seconds if auto-restart is enabled and 0
// otherwise. When it is set, the value is honoured including an explicit 0,
// even with auto-restart enabled; an unparseable ambient value is treated
// as 0. The flag's value can never fail to parse, since it is already a
// typed int by the time this function sees it.
func resolveKillOldDelay(isSet bool, flagSeconds int, enableAutoRestart bool) time.Duration {
	if isSet {
		return time.Duration(flagSeconds) * time.Second
	}
	if raw, ok := os.LookupEnv("KILL_OLD_DELAY"); ok {
		delay, _ := strconv.ParseInt(raw, 10, 0)
		return time.Duration(delay) * time.Second
	}
	if enableAutoRestart {
		return defaultKillOldDelayWithAutoRestart
	}
	return 0
}

package cli

import (
	"fmt"
	"math"
	"os"
	"strconv"
	"time"
)

// defaultAutoRestartInterval mirrors internal/supervisor's historical
// default (previously baked into the AUTO_RESTART_INTERVAL reader).
const defaultAutoRestartInterval = 360 * time.Second

const maxAutoRestartIntervalSeconds = int64(math.MaxInt64) / int64(time.Second)

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
// variable, otherwise the default. Present but invalid lifecycle variables
// return an error instead of silently selecting another value.
func resolveSettings(opts *options, isSet flagIsSet) (resolvedSettings, error) {
	var rs resolvedSettings
	var err error

	rs.envdir = resolveEnvdir(isSet("envdir"), opts.OptEnvdir)
	rs.enableAutoRestart, err = resolveEnableAutoRestart(isSet("enable-auto-restart"), opts.OptEnableAutoRestart)
	if err != nil {
		return resolvedSettings{}, err
	}
	rs.autoRestartInterval, err = resolveAutoRestartInterval(
		isSet("auto-restart-interval"),
		opts.OptAutoRestartInterval,
	)
	if err != nil {
		return resolvedSettings{}, err
	}
	rs.killOldDelay, err = resolveKillOldDelay(
		isSet("kill-old-delay"),
		opts.OptKillOldDelay,
		rs.enableAutoRestart,
	)
	if err != nil {
		return resolvedSettings{}, err
	}

	return rs, nil
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
// ENABLE_AUTO_RESTART parsed by strconv.ParseBool, otherwise false. A present
// but invalid ambient value is a configuration error.
func resolveEnableAutoRestart(isSet bool, flagValue bool) (bool, error) {
	if isSet {
		return flagValue, nil
	}
	value, ok := os.LookupEnv("ENABLE_AUTO_RESTART")
	if !ok {
		return false, nil
	}
	enabled, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("invalid ENABLE_AUTO_RESTART value %q: %w", value, err)
	}
	return enabled, nil
}

// resolveAutoRestartInterval preserves the historical flag behavior, while a
// present but invalid AUTO_RESTART_INTERVAL value is a configuration error.
// An absent variable uses the 360-second default.
func resolveAutoRestartInterval(isSet bool, flagSeconds int) (time.Duration, error) {
	if isSet {
		seconds := int64(flagSeconds)
		if seconds > 0 && seconds <= maxAutoRestartIntervalSeconds {
			return time.Duration(seconds) * time.Second, nil
		}
		return defaultAutoRestartInterval, nil
	}
	raw, ok := os.LookupEnv("AUTO_RESTART_INTERVAL")
	if !ok {
		return defaultAutoRestartInterval, nil
	}
	parsed, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid AUTO_RESTART_INTERVAL value %q: %w", raw, err)
	}
	if parsed <= 0 {
		return 0, fmt.Errorf("invalid AUTO_RESTART_INTERVAL value %q: must be greater than zero", raw)
	}
	if parsed > maxAutoRestartIntervalSeconds {
		return 0, fmt.Errorf(
			"invalid AUTO_RESTART_INTERVAL value %q: must not exceed %d",
			raw,
			maxAutoRestartIntervalSeconds,
		)
	}
	return time.Duration(parsed) * time.Second, nil
}

// resolveKillOldDelay matches internal/supervisor's old getKillOldDelay():
// when nothing sets the delay (neither the flag nor the ambient
// environment), the default is 5 seconds if auto-restart is enabled and 0
// otherwise. When it is set, the value is honoured including an explicit 0,
// even with auto-restart enabled. An unparseable ambient value is a
// configuration error. The flag's value can never fail to parse, since it is
// already a typed int by the time this function sees it.
func resolveKillOldDelay(isSet bool, flagSeconds int, enableAutoRestart bool) (time.Duration, error) {
	if isSet {
		return time.Duration(flagSeconds) * time.Second, nil
	}
	if raw, ok := os.LookupEnv("KILL_OLD_DELAY"); ok {
		delay, err := strconv.ParseInt(raw, 10, 0)
		if err != nil {
			return 0, fmt.Errorf("invalid KILL_OLD_DELAY value %q: %w", raw, err)
		}
		return time.Duration(delay) * time.Second, nil
	}
	if enableAutoRestart {
		return defaultKillOldDelayWithAutoRestart, nil
	}
	return 0, nil
}

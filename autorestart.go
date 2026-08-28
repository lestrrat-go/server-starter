package starter

import (
	"os"
	"strconv"
	"time"
)

func autoRestartEnabled() bool {
	value, ok := os.LookupEnv("ENABLE_AUTO_RESTART")
	if !ok {
		return false
	}
	enabled, _ := strconv.ParseBool(value)
	return enabled || value == "1"
}

func autoRestartInterval() time.Duration {
	interval := 360
	if value, ok := os.LookupEnv("AUTO_RESTART_INTERVAL"); ok {
		if parsed, err := strconv.Atoi(value); err == nil && parsed > 0 {
			interval = parsed
		}
	}
	return time.Duration(interval) * time.Second
}

func getKillOldDelay() time.Duration {
	autoRestart, _ := strconv.ParseBool(os.Getenv("ENABLE_AUTO_RESTART"))

	v, ok := os.LookupEnv("KILL_OLD_DELAY")
	if !ok {
		if autoRestart {
			return 5 * time.Second
		}
		return 0
	}

	// KILL_OLD_DELAY is set: honour it, including an explicit 0, even when
	// auto-restart is enabled. An unparseable value is treated as 0,
	// consistent with this function's existing tolerance for bad input.
	delay, _ := strconv.ParseInt(v, 10, 0)

	return time.Duration(delay) * time.Second
}

package cli

// This test lives in the internal `cli` package (not `cli_test`) because
// resolveSettings and its helpers are unexported.

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// setEnvUnset ensures key is unset for the duration of the test, restoring
// whatever value (or absence) it had beforehand once the test completes.
// t.Setenv cannot express "absent", and several of these resolvers
// distinguish absent from set-to-empty via os.LookupEnv.
func setEnvUnset(t *testing.T, key string) {
	t.Helper()

	original, hadOriginal := os.LookupEnv(key)
	require.NoError(t, os.Unsetenv(key))
	t.Cleanup(func() {
		if hadOriginal {
			t.Setenv(key, original)
			return
		}
		require.NoError(t, os.Unsetenv(key))
	})
}

func alwaysSet(names ...string) flagIsSet {
	set := make(map[string]struct{}, len(names))
	for _, n := range names {
		set[n] = struct{}{}
	}
	return func(long string) bool {
		_, ok := set[long]
		return ok
	}
}

func noneSet() flagIsSet {
	return func(string) bool { return false }
}

func TestResolveEnvdir(t *testing.T) {
	t.Run("flag set wins over ambient", func(t *testing.T) {
		t.Setenv("ENVDIR", "/from/env")
		require.Equal(t, "/from/flag", resolveEnvdir(true, "/from/flag"))
	})
	t.Run("falls back to ambient when flag unset", func(t *testing.T) {
		t.Setenv("ENVDIR", "/from/env")
		require.Equal(t, "/from/env", resolveEnvdir(false, ""))
	})
	t.Run("defaults to empty", func(t *testing.T) {
		setEnvUnset(t, "ENVDIR")
		require.Equal(t, "", resolveEnvdir(false, ""))
	})
}

func TestResolveEnableAutoRestart(t *testing.T) {
	testCases := []struct {
		name     string
		isSet    bool
		flagVal  bool
		envSet   bool
		envVal   string
		expected bool
	}{
		{name: "flag true wins over ambient", isSet: true, flagVal: true, envSet: true, envVal: "0", expected: true},
		{name: "flag false wins over ambient", isSet: true, flagVal: false, envSet: true, envVal: "1", expected: false},
		{name: "ambient literal 1", isSet: false, envSet: true, envVal: "1", expected: true},
		{name: "ambient true", isSet: false, envSet: true, envVal: "true", expected: true},
		{name: "ambient false", isSet: false, envSet: true, envVal: "false", expected: false},
		{name: "ambient garbage", isSet: false, envSet: true, envVal: "banana", expected: false},
		{name: "ambient unset", isSet: false, envSet: false, expected: false},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.envSet {
				t.Setenv("ENABLE_AUTO_RESTART", tc.envVal)
			} else {
				setEnvUnset(t, "ENABLE_AUTO_RESTART")
			}
			require.Equal(t, tc.expected, resolveEnableAutoRestart(tc.isSet, tc.flagVal))
		})
	}
}

func TestResolveAutoRestartInterval(t *testing.T) {
	testCases := []struct {
		name     string
		isSet    bool
		flagVal  int
		envSet   bool
		envVal   string
		expected time.Duration
	}{
		{name: "flag positive", isSet: true, flagVal: 42, expected: 42 * time.Second},
		{name: "flag zero falls back to default", isSet: true, flagVal: 0, expected: defaultAutoRestartInterval},
		{name: "flag negative falls back to default", isSet: true, flagVal: -1, expected: defaultAutoRestartInterval},
		{name: "ambient positive", isSet: false, envSet: true, envVal: "7", expected: 7 * time.Second},
		{name: "ambient zero falls back to default", isSet: false, envSet: true, envVal: "0", expected: defaultAutoRestartInterval},
		{name: "ambient unparseable falls back to default", isSet: false, envSet: true, envVal: "nope", expected: defaultAutoRestartInterval},
		{name: "ambient unset defaults", isSet: false, envSet: false, expected: defaultAutoRestartInterval},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.envSet {
				t.Setenv("AUTO_RESTART_INTERVAL", tc.envVal)
			} else {
				setEnvUnset(t, "AUTO_RESTART_INTERVAL")
			}
			require.Equal(t, tc.expected, resolveAutoRestartInterval(tc.isSet, tc.flagVal))
		})
	}
}

func TestResolveKillOldDelay(t *testing.T) {
	testCases := []struct {
		name        string
		isSet       bool
		flagVal     int
		envSet      bool
		envVal      string
		autoRestart bool
		expected    time.Duration
	}{
		{name: "unset, auto-restart off", isSet: false, envSet: false, autoRestart: false, expected: 0},
		{name: "unset, auto-restart on", isSet: false, envSet: false, autoRestart: true, expected: 5 * time.Second},
		{name: "flag 0, auto-restart on", isSet: true, flagVal: 0, autoRestart: true, expected: 0},
		{name: "flag 0, auto-restart off", isSet: true, flagVal: 0, autoRestart: false, expected: 0},
		{name: "flag 3", isSet: true, flagVal: 3, autoRestart: false, expected: 3 * time.Second},
		{name: "ambient 0, auto-restart on", isSet: false, envSet: true, envVal: "0", autoRestart: true, expected: 0},
		{name: "ambient 3", isSet: false, envSet: true, envVal: "3", autoRestart: false, expected: 3 * time.Second},
		{name: "ambient unparseable treated as 0", isSet: false, envSet: true, envVal: "nope", autoRestart: true, expected: 0},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.envSet {
				t.Setenv("KILL_OLD_DELAY", tc.envVal)
			} else {
				setEnvUnset(t, "KILL_OLD_DELAY")
			}
			require.Equal(t, tc.expected, resolveKillOldDelay(tc.isSet, tc.flagVal, tc.autoRestart))
		})
	}
}

// TestResolveSettings covers the whole precedence pipeline together,
// including the one place a later setting depends on an earlier one:
// killOldDelay's default depends on the *resolved* enableAutoRestart, not
// the raw flag value.
func TestResolveSettings(t *testing.T) {
	t.Run("all defaults when nothing set", func(t *testing.T) {
		setEnvUnset(t, "ENVDIR")
		setEnvUnset(t, "ENABLE_AUTO_RESTART")
		setEnvUnset(t, "AUTO_RESTART_INTERVAL")
		setEnvUnset(t, "KILL_OLD_DELAY")

		got := resolveSettings(&options{}, noneSet())
		require.Equal(t, resolvedSettings{
			envdir:              "",
			enableAutoRestart:   false,
			autoRestartInterval: defaultAutoRestartInterval,
			killOldDelay:        0,
		}, got)
	})

	t.Run("enable-auto-restart flag drives kill-old-delay default", func(t *testing.T) {
		setEnvUnset(t, "KILL_OLD_DELAY")

		opts := &options{OptEnableAutoRestart: true}
		got := resolveSettings(opts, alwaysSet("enable-auto-restart"))
		require.True(t, got.enableAutoRestart)
		require.Equal(t, 5*time.Second, got.killOldDelay)
	})

	t.Run("flags take precedence over ambient environment", func(t *testing.T) {
		t.Setenv("ENVDIR", "/from/env")
		t.Setenv("ENABLE_AUTO_RESTART", "0")
		t.Setenv("AUTO_RESTART_INTERVAL", "99")
		t.Setenv("KILL_OLD_DELAY", "99")

		opts := &options{
			OptEnvdir:              "/from/flag",
			OptEnableAutoRestart:   true,
			OptAutoRestartInterval: 12,
			OptKillOldDelay:        7,
		}
		got := resolveSettings(opts, alwaysSet("envdir", "enable-auto-restart", "auto-restart-interval", "kill-old-delay"))
		require.Equal(t, resolvedSettings{
			envdir:              "/from/flag",
			enableAutoRestart:   true,
			autoRestartInterval: 12 * time.Second,
			killOldDelay:        7 * time.Second,
		}, got)
	})
}

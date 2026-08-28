package starter

// This test lives in the internal `starter` package (not `starter_test`)
// because getKillOldDelay is unexported.

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// setEnvUnset ensures key is unset for the duration of the test, restoring
// whatever value (or absence) it had beforehand once the test completes.
// t.Setenv cannot express "absent" (getKillOldDelay distinguishes absent
// from set-to-empty via os.LookupEnv), so this restores manually.
func setEnvUnset(t *testing.T, key string) {
	t.Helper()

	original, hadOriginal := os.LookupEnv(key)
	require.NoError(t, os.Unsetenv(key))
	t.Cleanup(func() {
		if hadOriginal {
			//nolint:usetesting // restoring in Cleanup, not setting for the test itself
			os.Setenv(key, original)
			return
		}
		os.Unsetenv(key)
	})
}

func TestGetKillOldDelay(t *testing.T) {
	testCases := []struct {
		name        string
		envSet      bool
		envValue    string
		autoRestart bool
		expected    time.Duration
	}{
		{name: "unset, auto-restart off", envSet: false, autoRestart: false, expected: 0},
		{name: "unset, auto-restart on", envSet: false, autoRestart: true, expected: 5 * time.Second},
		{name: "0, auto-restart off", envSet: true, envValue: "0", autoRestart: false, expected: 0},
		{name: "0, auto-restart on", envSet: true, envValue: "0", autoRestart: true, expected: 0},
		{name: "3, auto-restart off", envSet: true, envValue: "3", autoRestart: false, expected: 3 * time.Second},
		{name: "3, auto-restart on", envSet: true, envValue: "3", autoRestart: true, expected: 3 * time.Second},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.envSet {
				t.Setenv("KILL_OLD_DELAY", tc.envValue)
			} else {
				setEnvUnset(t, "KILL_OLD_DELAY")
			}

			if tc.autoRestart {
				t.Setenv("ENABLE_AUTO_RESTART", "1")
			} else {
				setEnvUnset(t, "ENABLE_AUTO_RESTART")
			}

			require.Equal(t, tc.expected, getKillOldDelay())
		})
	}
}

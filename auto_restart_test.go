package starter

import (
	"testing"
	"time"
)

func TestAutoRestartSettings(t *testing.T) {
	t.Setenv("ENABLE_AUTO_RESTART", "1")
	t.Setenv("AUTO_RESTART_INTERVAL", "7")
	if !autoRestartEnabled() {
		t.Fatal("auto-restart should be enabled")
	}
	if got := autoRestartInterval(); got != 7*time.Second {
		t.Fatalf("auto-restart interval = %s", got)
	}

	t.Setenv("ENABLE_AUTO_RESTART", "false")
	t.Setenv("AUTO_RESTART_INTERVAL", "")
	if autoRestartEnabled() {
		t.Fatal("auto-restart should be disabled")
	}
	if got := autoRestartInterval(); got != 360*time.Second {
		t.Fatalf("default auto-restart interval = %s", got)
	}
}

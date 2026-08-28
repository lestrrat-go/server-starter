package control

import (
	"context"
	"errors"
	"fmt"
	"os"
	"syscall"
	"time"

	"github.com/lestrrat-go/server-starter/v2/internal/statefile"
)

// pollInterval is how often Stop and Restart re-check for completion while
// waiting for the target process to react to a signal.
const pollInterval = 20 * time.Millisecond

// Stop reads the pid from pidPath, sends SIGTERM, and waits for the process
// to exit. The caller controls how long to wait via ctx; a typical caller
// wraps ctx with a timeout.
func Stop(ctx context.Context, pidPath string) error {
	pid, err := statefile.ReadPID(pidPath)
	if err != nil {
		return err
	}
	if err := signalProcess(pid, syscall.SIGTERM); err != nil && !errors.Is(err, syscall.ESRCH) {
		return err
	}
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timed out waiting for process %d to stop: %w", pid, ctx.Err())
		default:
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("timed out waiting for process %d to stop: %w", pid, ctx.Err())
		case <-ticker.C:
		}
		f, err := os.OpenFile(pidPath, os.O_RDWR, 0644)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err == nil {
			err = statefile.TryLock(f)
			f.Close()
			if err == nil {
				return nil
			}
		}
	}
}

// Restart reads the pid from pidPath, sends SIGHUP, and waits until the
// server(s) of the older generation(s) die by monitoring the contents of
// statusPath. The caller controls how long to wait via ctx; a typical
// caller wraps ctx with a timeout.
func Restart(ctx context.Context, pidPath, statusPath string) error {
	if statusPath == "" {
		return fmt.Errorf("--status-file is required with --restart")
	}
	pid, err := statefile.ReadPID(pidPath)
	if err != nil {
		return err
	}
	previous, err := statefile.ReadStatus(statusPath)
	if err != nil {
		return err
	}
	if err := signalProcess(pid, syscall.SIGHUP); err != nil {
		return err
	}
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timed out waiting for restart: %w", ctx.Err())
		default:
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("timed out waiting for restart: %w", ctx.Err())
		case <-ticker.C:
		}
		current, err := statefile.ReadStatus(statusPath)
		if err == nil && generationAdvanced(previous, current) && oldWorkersGone(previous, current) {
			return nil
		}
	}
}

func generationAdvanced(previous, current map[int]int) bool {
	for generation := range current {
		if _, ok := previous[generation]; !ok {
			return true
		}
	}
	return false
}

func oldWorkersGone(previous, current map[int]int) bool {
	for generation, pid := range previous {
		if current[generation] == pid {
			return false
		}
	}
	return true
}

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
	return stopWithOpenRunningPID(ctx, pidPath, statefile.OpenRunningPID)
}

func stopWithOpenRunningPID(
	ctx context.Context,
	pidPath string,
	openRunningPID func(string) (*statefile.RunningPID, error),
) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("stop cancelled before signalling supervisor: %w", err)
	}
	running, err := openRunningPID(pidPath)
	if err != nil {
		return err
	}
	defer running.Close()
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("stop cancelled before signalling supervisor: %w", err)
	}
	pid := running.PID()
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
		exited, err := running.Exited()
		if err != nil {
			return fmt.Errorf("failed to check process %d: %w", pid, err)
		}
		if exited {
			return nil
		}
	}
}

// processStopped is retained for the polling unit tests and for callers that
// need to check a legacy lock without opening a RunningPID handle.
func processStopped(pidPath string, tryLock func(*os.File) error) (bool, error) {
	f, err := os.OpenFile(pidPath, os.O_RDWR, 0)
	if errors.Is(err, os.ErrNotExist) {
		return true, nil
	}
	if err != nil {
		return false, fmt.Errorf("failed to open pid file %q while waiting for process to stop: %w", pidPath, err)
	}
	lockErr := tryLock(f)
	closeErr := f.Close()
	if lockErr != nil && !errors.Is(lockErr, syscall.EWOULDBLOCK) {
		return false, fmt.Errorf("failed to check pid file %q while waiting for process to stop: %w", pidPath, lockErr)
	}
	if closeErr != nil {
		return false, fmt.Errorf("failed to close pid file %q while waiting for process to stop: %w", pidPath, closeErr)
	}
	return lockErr == nil, nil
}

// Restart reads the pid from pidPath, sends SIGHUP, and waits until the
// server(s) of the older generation(s) die by monitoring the contents of
// statusPath. The caller controls how long to wait via ctx; a typical
// caller wraps ctx with a timeout.
func Restart(ctx context.Context, pidPath, statusPath string) error {
	if statusPath == "" {
		return fmt.Errorf("--status-file is required with --restart")
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("restart cancelled before signalling supervisor: %w", err)
	}
	running, err := statefile.OpenRunningPID(pidPath)
	if err != nil {
		return err
	}
	defer running.Close()
	pid := running.PID()
	previous, err := statefile.ReadStatus(ctx, statusPath)
	if err != nil {
		return fmt.Errorf("failed to read status file %q before restart: %w", statusPath, err)
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
		current, err := statefile.ReadStatus(ctx, statusPath)
		if err != nil {
			return fmt.Errorf("failed to read status file %q while waiting for restart: %w", statusPath, err)
		}
		if generationAdvanced(previous, current) && oldWorkersGone(previous, current) {
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

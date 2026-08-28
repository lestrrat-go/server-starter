package control

import (
	"errors"
	"fmt"
	"os"
	"syscall"
	"time"

	"github.com/lestrrat-go/server-starter/v2/internal/statefile"
)

const controlTimeout = 30 * time.Second

// Stop reads the pid from pidPath, sends SIGTERM, and waits for the process
// to exit.
func Stop(pidPath string) error {
	pid, err := statefile.ReadPID(pidPath)
	if err != nil {
		return err
	}
	if err := signalProcess(pid, syscall.SIGTERM); err != nil && !errors.Is(err, syscall.ESRCH) {
		return err
	}
	deadline := time.Now().Add(controlTimeout)
	for time.Now().Before(deadline) {
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
		time.Sleep(20 * time.Millisecond)
	}
	return fmt.Errorf("timed out waiting for process %d to stop", pid)
}

// Restart reads the pid from pidPath, sends SIGHUP, and waits until the
// server(s) of the older generation(s) die by monitoring the contents of
// statusPath.
func Restart(pidPath, statusPath string) error {
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
	deadline := time.Now().Add(controlTimeout)
	for time.Now().Before(deadline) {
		current, err := statefile.ReadStatus(statusPath)
		if err == nil && generationAdvanced(previous, current) && oldWorkersGone(previous, current) {
			return nil
		}
		time.Sleep(20 * time.Millisecond)
	}
	return fmt.Errorf("timed out waiting for restart")
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

package main

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const controlTimeout = 30 * time.Second

func readPID(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	value := strings.TrimSpace(string(data))
	pid, err := strconv.Atoi(value)
	if err != nil || pid <= 0 {
		return 0, fmt.Errorf("invalid pid file %q", path)
	}
	return pid, nil
}

func stopServer(pidPath string) error {
	pid, err := readPID(pidPath)
	if err != nil {
		return err
	}
	if err := syscall.Kill(pid, syscall.SIGTERM); err != nil && !errors.Is(err, syscall.ESRCH) {
		return err
	}
	deadline := time.Now().Add(controlTimeout)
	for time.Now().Before(deadline) {
		f, err := os.OpenFile(pidPath, os.O_RDWR, 0644)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err == nil {
			err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
			f.Close()
			if err == nil {
				return nil
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	return fmt.Errorf("timed out waiting for process %d to stop", pid)
}

func readStatus(path string) (map[int]int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	status := make(map[int]int)
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.Split(line, ":")
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid status line %q", line)
		}
		generation, err := strconv.Atoi(parts[0])
		if err != nil {
			return nil, err
		}
		pid, err := strconv.Atoi(parts[1])
		if err != nil {
			return nil, err
		}
		status[generation] = pid
	}
	return status, nil
}

func restartServer(pidPath, statusPath string) error {
	if statusPath == "" {
		return fmt.Errorf("--status-file is required with --restart")
	}
	pid, err := readPID(pidPath)
	if err != nil {
		return err
	}
	previous, err := readStatus(statusPath)
	if err != nil {
		return err
	}
	if err := syscall.Kill(pid, syscall.SIGHUP); err != nil {
		return err
	}
	deadline := time.Now().Add(controlTimeout)
	for time.Now().Before(deadline) {
		current, err := readStatus(statusPath)
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

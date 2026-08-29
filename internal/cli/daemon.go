package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

const (
	daemonizedEnv       = "SERVER_STARTER_DAEMONIZED"
	daemonReadinessEnv  = "SERVER_STARTER_DAEMON_READY_FD"
	daemonReadinessFD   = 3
	daemonReady         = byte(1)
	daemonFailed        = byte(2)
	maxDaemonStatusSize = 64 * 1024
)

func daemonize() error {
	attr, err := daemonSysProcAttr()
	if err != nil {
		return err
	}
	reader, writer, err := os.Pipe()
	if err != nil {
		return fmt.Errorf("create daemon readiness pipe: %w", err)
	}
	defer reader.Close()

	// The daemonized child deliberately outlives this process, so its
	// lifetime is not tied to any cancellable context.
	cmd := exec.CommandContext(context.Background(), os.Args[0], os.Args[1:]...)
	cmd.Env = daemonEnvironment()
	cmd.SysProcAttr = attr
	cmd.Stdin = nil
	cmd.ExtraFiles = []*os.File{writer}
	if err := cmd.Start(); err != nil {
		writer.Close()
		return err
	}
	if err := writer.Close(); err != nil {
		releaseErr := cmd.Process.Release()
		return errors.Join(fmt.Errorf("close daemon readiness pipe: %w", err), releaseErr)
	}

	statusErr := readDaemonStatus(reader)
	releaseErr := cmd.Process.Release()
	if statusErr != nil {
		return statusErr
	}
	return releaseErr
}

func daemonEnvironment() []string {
	environ := os.Environ()
	childEnv := make([]string, 0, len(environ)+2)
	for _, entry := range environ {
		if strings.HasPrefix(entry, daemonizedEnv+"=") || strings.HasPrefix(entry, daemonReadinessEnv+"=") {
			continue
		}
		childEnv = append(childEnv, entry)
	}
	return append(childEnv,
		daemonizedEnv+"=1",
		daemonReadinessEnv+"="+strconv.Itoa(daemonReadinessFD),
	)
}

type daemonReadiness struct {
	file *os.File
}

func (r *daemonReadiness) active() bool {
	return r.file != nil
}

func childDaemonReadiness() (*daemonReadiness, error) {
	if os.Getenv(daemonizedEnv) != "1" {
		return &daemonReadiness{}, nil
	}
	value, ok := os.LookupEnv(daemonReadinessEnv)
	if !ok {
		return &daemonReadiness{}, nil
	}
	if err := os.Unsetenv(daemonReadinessEnv); err != nil {
		return nil, fmt.Errorf("clear daemon readiness descriptor: %w", err)
	}
	fd, err := strconv.Atoi(value)
	if err != nil || fd < daemonReadinessFD {
		return nil, fmt.Errorf("invalid daemon readiness descriptor %q", value)
	}
	file := os.NewFile(uintptr(fd), "daemon-readiness")
	if file == nil {
		return nil, fmt.Errorf("open daemon readiness descriptor %d", fd)
	}
	closeDaemonReadinessOnExec(uintptr(fd))
	return &daemonReadiness{file: file}, nil
}

func (r *daemonReadiness) ready() error {
	if r.file == nil {
		return nil
	}
	return r.report([]byte{daemonReady})
}

func (r *daemonReadiness) failed(err error) {
	if r.file == nil {
		return
	}
	message := err.Error()
	if len(message) >= maxDaemonStatusSize {
		message = message[:maxDaemonStatusSize-1]
	}
	payload := append([]byte{daemonFailed}, message...)
	_ = r.report(payload)
}

func (r *daemonReadiness) report(payload []byte) error {
	_, writeErr := r.file.Write(payload)
	closeErr := r.file.Close()
	r.file = nil
	return errors.Join(writeErr, closeErr)
}

func readDaemonStatus(reader io.Reader) error {
	payload, err := io.ReadAll(io.LimitReader(reader, maxDaemonStatusSize+1))
	if err != nil {
		return fmt.Errorf("read daemon startup status: %w", err)
	}
	if len(payload) == 0 {
		return fmt.Errorf("daemon exited before reporting startup status")
	}
	if len(payload) > maxDaemonStatusSize {
		return fmt.Errorf("daemon startup status exceeds %d bytes", maxDaemonStatusSize)
	}
	switch payload[0] {
	case daemonReady:
		if len(payload) != 1 {
			return fmt.Errorf("invalid daemon readiness response")
		}
		return nil
	case daemonFailed:
		if len(payload) == 1 {
			return fmt.Errorf("daemon startup failed")
		}
		return fmt.Errorf("daemon startup failed: %s", payload[1:])
	default:
		return fmt.Errorf("invalid daemon startup response")
	}
}

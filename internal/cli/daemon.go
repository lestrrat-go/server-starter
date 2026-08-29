package cli

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

const (
	daemonizedEnv          = "SERVER_STARTER_DAEMONIZED"
	daemonReadinessEnv     = "SERVER_STARTER_DAEMON_READY_FD"
	daemonReadinessFD      = 3
	daemonReady            = byte(1)
	daemonFailed           = byte(2)
	daemonStatusHeaderSize = 1 + 8
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
	return r.report(daemonReady, nil)
}

func (r *daemonReadiness) failed(err error) {
	if r.file == nil {
		return
	}
	_ = r.report(daemonFailed, []byte(err.Error()))
}

func (r *daemonReadiness) report(status byte, body []byte) error {
	writeErr := writeDaemonStatus(r.file, status, body)
	closeErr := r.file.Close()
	r.file = nil
	return errors.Join(writeErr, closeErr)
}

func writeDaemonStatus(writer io.Writer, status byte, body []byte) error {
	// A fixed-width length distinguishes a complete body from a pipe that
	// closes while the child is still reporting its startup result.
	header := [daemonStatusHeaderSize]byte{status}
	binary.BigEndian.PutUint64(header[1:], uint64(len(body)))
	if err := writeDaemonStatusBytes(writer, header[:]); err != nil {
		return err
	}
	return writeDaemonStatusBytes(writer, body)
}

func writeDaemonStatusBytes(writer io.Writer, data []byte) error {
	for len(data) > 0 {
		n, err := writer.Write(data)
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
		data = data[n:]
	}
	return nil
}

func readDaemonStatus(reader io.Reader) error {
	var header [daemonStatusHeaderSize]byte
	n, err := io.ReadFull(reader, header[:])
	if err != nil {
		if n == 0 && errors.Is(err, io.EOF) {
			return fmt.Errorf("daemon exited before reporting startup status")
		}
		return fmt.Errorf("read daemon startup status: %w", err)
	}
	bodySize := binary.BigEndian.Uint64(header[1:])
	switch header[0] {
	case daemonReady:
		if bodySize != 0 {
			return fmt.Errorf("invalid daemon readiness response")
		}
		hasTrailingData, err := daemonStatusHasTrailingData(reader)
		if err != nil {
			return err
		}
		if hasTrailingData {
			return fmt.Errorf("invalid daemon readiness response")
		}
		return nil
	case daemonFailed:
		if bodySize > uint64(^uint(0)>>1) {
			return fmt.Errorf("daemon startup status length exceeds platform capacity")
		}
		body := make([]byte, int(bodySize))
		if _, err := io.ReadFull(reader, body); err != nil {
			return fmt.Errorf("read daemon startup status: %w", err)
		}
		hasTrailingData, err := daemonStatusHasTrailingData(reader)
		if err != nil {
			return err
		}
		if hasTrailingData {
			return fmt.Errorf("invalid daemon startup response")
		}
		if len(body) == 0 {
			return fmt.Errorf("daemon startup failed")
		}
		return fmt.Errorf("daemon startup failed: %s", body)
	default:
		return fmt.Errorf("invalid daemon startup response")
	}
}

func daemonStatusHasTrailingData(reader io.Reader) (bool, error) {
	var extra [1]byte
	n, err := io.ReadFull(reader, extra[:])
	if n > 0 {
		return true, nil
	}
	if errors.Is(err, io.EOF) {
		return false, nil
	}
	return false, fmt.Errorf("read daemon startup status: %w", err)
}

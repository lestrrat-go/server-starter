package cli

import (
	"context"
	"os"
	"os/exec"
)

func daemonize() error {
	attr, err := daemonSysProcAttr()
	if err != nil {
		return err
	}
	// The daemonized child deliberately outlives this process, so its
	// lifetime is not tied to any cancellable context.
	cmd := exec.CommandContext(context.Background(), os.Args[0], os.Args[1:]...)
	cmd.Env = append(os.Environ(), "SERVER_STARTER_DAEMONIZED=1")
	cmd.SysProcAttr = attr
	cmd.Stdin = nil
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

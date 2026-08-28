package cli

import (
	"os"
	"os/exec"
)

func daemonize() error {
	attr, err := daemonSysProcAttr()
	if err != nil {
		return err
	}
	cmd := exec.Command(os.Args[0], os.Args[1:]...)
	cmd.Env = append(os.Environ(), "SERVER_STARTER_DAEMONIZED=1")
	cmd.SysProcAttr = attr
	cmd.Stdin = nil
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

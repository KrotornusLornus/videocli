package cli

import (
	"os"
	"os/exec"
	"syscall"
)

// lookPath envuelve exec.LookPath.
func lookPath(name string) (string, error) {
	return exec.LookPath(name)
}

// startDetached lanza cmd con args en segundo plano (fire-and-forget).
func startDetached(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	devnull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err == nil {
		cmd.Stdout = devnull
		cmd.Stderr = devnull
		defer devnull.Close()
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}

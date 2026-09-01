//go:build !windows

package main

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

func handledSignals() []os.Signal {
	return []os.Signal{os.Interrupt, syscall.SIGTERM, syscall.SIGUSR1}
}

func isShutdownSignal(sig os.Signal) bool {
	return sig == os.Interrupt || sig == syscall.SIGTERM
}

func isRestartSignal(sig os.Signal) bool {
	return sig == syscall.SIGUSR1
}

func stopProcess(cmd *exec.Cmd) error {
	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return err
	}
	return nil
}

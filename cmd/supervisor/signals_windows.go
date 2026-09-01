//go:build windows

package main

import (
	"errors"
	"os"
	"os/exec"
)

func handledSignals() []os.Signal {
	return []os.Signal{os.Interrupt}
}

func isShutdownSignal(sig os.Signal) bool {
	return sig == os.Interrupt
}

func isRestartSignal(sig os.Signal) bool {
	return false
}

func stopProcess(cmd *exec.Cmd) error {
	if err := cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return err
	}
	return nil
}

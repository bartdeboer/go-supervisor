package main

import (
	"errors"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/bartdeboer/go-supervisor/initcfg"
	"github.com/bartdeboer/go-supervisor/internal/defaults"
)

func main() {
	tuneRuntime()

	configPath, park, fallback, err := parseArgs(os.Args[1:])
	if err != nil {
		fatal(err)
	}
	cfg, err := initcfg.ReadConfigFile(configPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			cfg = initcfg.Config{}
		} else {
			fatal(err)
		}
	}
	fmt.Fprintf(os.Stderr, "supervisord: loaded %d services\n", len(cfg.Services))
	if park {
		fmt.Fprintln(os.Stderr, "supervisord: park mode requested")
	}
	if len(fallback) > 0 {
		fmt.Fprintf(os.Stderr, "supervisord: fallback command configured: %v\n", fallback)
	}
	if park {
		waitForShutdownSignal()
	}
	// Full runtime supervision intentionally follows in the next slice.
}

func tuneRuntime() {
	// supervisord is an event loop, not a CPU-bound worker. Keeping a single P
	// trims the parked PID-1 footprint while still allowing operators to opt out.
	if os.Getenv("GOMAXPROCS") == "" {
		runtime.GOMAXPROCS(1)
	}
}

func waitForShutdownSignal() {
	signals := make(chan os.Signal, 8)
	signal.Notify(signals, syscall.SIGTERM, syscall.SIGINT, syscall.SIGQUIT, syscall.SIGCHLD)
	for {
		sig := <-signals
		switch sig {
		case syscall.SIGCHLD:
			reapExitedChildren()
		default:
			fmt.Fprintf(os.Stderr, "supervisord: received %s; exiting\n", sig)
			reapExitedChildren()
			return
		}
	}
}

func reapExitedChildren() {
	for {
		var status syscall.WaitStatus
		pid, err := syscall.Wait4(-1, &status, syscall.WNOHANG, nil)
		if pid > 0 {
			fmt.Fprintf(os.Stderr, "supervisord: reaped child pid=%d status=%d\n", pid, status)
			continue
		}
		if pid == 0 || errors.Is(err, syscall.ECHILD) {
			return
		}
		fmt.Fprintf(os.Stderr, "supervisord: reap error: %v\n", err)
		return
	}
}

func parseArgs(args []string) (configPath string, park bool, fallback []string, err error) {
	park = true
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--config":
			i++
			if i >= len(args) || args[i] == "" {
				return "", false, nil, fmt.Errorf("missing --config value")
			}
			configPath = args[i]
		case "--park":
			park = true
		case "--no-park":
			park = false
		case "--":
			fallback = append([]string(nil), args[i+1:]...)
			i = len(args)
		case "--help", "-h":
			return "", false, nil, errHelp{}
		default:
			return "", false, nil, fmt.Errorf("unknown argument: %s", args[i])
		}
	}
	configPath = defaults.ConfigPathFrom(configPath, os.Getenv)
	return configPath, park, fallback, nil
}

type errHelp struct{}

func (errHelp) Error() string { return usage() }

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	fmt.Fprintln(os.Stderr, usage())
	os.Exit(2)
}

func usage() string {
	return "usage: supervisord [--config <path>] [--no-park] [-- <fallback> [args...]]"
}

package main

import (
	"errors"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
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
	warn("loaded " + strconv.Itoa(len(cfg.Services)) + " services")
	if park {
		warn("park mode requested")
	}
	if len(fallback) > 0 {
		warn("fallback command configured: " + strings.Join(fallback, " "))
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
			warn("received " + sig.String() + "; exiting")
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
			warn("reaped child pid=" + strconv.Itoa(pid) + " status=" + strconv.Itoa(int(status)))
			continue
		}
		if pid == 0 || errors.Is(err, syscall.ECHILD) {
			return
		}
		warn("reap error: " + err.Error())
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
				return "", false, nil, errors.New("missing --config value")
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
			return "", false, nil, errors.New("unknown argument: " + args[i])
		}
	}
	configPath = defaults.ConfigPathFrom(configPath, os.Getenv)
	return configPath, park, fallback, nil
}

type errHelp struct{}

func (errHelp) Error() string { return usage() }

func fatal(err error) {
	warn(err.Error())
	_, _ = os.Stderr.WriteString(usage() + "\n")
	os.Exit(2)
}

func warn(s string) {
	_, _ = os.Stderr.WriteString("supervisord: " + s + "\n")
}

func usage() string {
	return "usage: supervisord [--config <path>] [--no-park] [-- <fallback> [args...]]"
}

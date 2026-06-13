package main

import (
	"errors"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	supervisor "github.com/bartdeboer/go-supervisor"
)

const (
	defaultStopTimeout = 5 * time.Second
	restartDelay       = time.Second
	maxRestartDelay    = 30 * time.Second
	stableRuntime      = 10 * time.Second
)

func main() {
	tuneRuntime()

	configPath, park, fallback, err := parseArgs(os.Args[1:])
	if err != nil {
		fatal(err)
	}
	cfg := loadConfig(configPath, park)
	warn("loaded " + strconv.Itoa(len(cfg.Services)) + " services")
	if park {
		warn("park mode requested")
	}
	if len(fallback) > 0 {
		warn("fallback command configured: " + strings.Join(fallback, " "))
	}
	if park {
		runDaemon(configPath, cfg)
	}
}

func tuneRuntime() {
	// supervisord is an event loop, not a CPU-bound worker. Keeping a single P
	// trims the parked PID-1 footprint while still allowing operators to opt out.
	if os.Getenv("GOMAXPROCS") == "" {
		runtime.GOMAXPROCS(1)
	}
}

func loadConfig(path string, failOpen bool) supervisor.Config {
	cfg, err := supervisor.ReadConfigFile(path)
	if err == nil {
		return cfg
	}
	if errors.Is(err, os.ErrNotExist) {
		return supervisor.Config{}
	}
	if failOpen {
		warn("could not read config; parking with no services: " + err.Error())
		return supervisor.Config{}
	}
	fatal(err)
	return supervisor.Config{}
}

func reloadConfig(path string) (supervisor.Config, bool) {
	cfg, err := supervisor.ReadConfigFile(path)
	if err == nil {
		return cfg, true
	}
	if errors.Is(err, os.ErrNotExist) {
		return supervisor.Config{}, true
	}
	warn("could not reload config; keeping current services: " + err.Error())
	return supervisor.Config{}, false
}

type daemon struct {
	configPath string
	services   map[string]*serviceState
	byPID      map[int]*serviceState
	restarts   chan string
	stopping   bool
}

type serviceState struct {
	spec            supervisor.Service
	pid             int
	desiredStop     bool
	startedAt       time.Time
	restartFailures int
}

func runDaemon(configPath string, cfg supervisor.Config) {
	d := &daemon{
		configPath: configPath,
		services:   map[string]*serviceState{},
		byPID:      map[int]*serviceState{},
		restarts:   make(chan string, supervisor.MaxServices),
	}
	d.applyConfig(cfg)

	signals := make(chan os.Signal, 16)
	signal.Notify(signals, syscall.SIGTERM, syscall.SIGINT, syscall.SIGQUIT, syscall.SIGHUP, syscall.SIGCHLD)
	for {
		select {
		case sig := <-signals:
			switch sig {
			case syscall.SIGCHLD:
				d.reapExitedChildren()
			case syscall.SIGHUP:
				warn("received hangup; reloading config")
				cfg, ok := reloadConfig(configPath)
				if ok {
					d.applyConfig(cfg)
				}
			default:
				warn("received " + sig.String() + "; stopping services")
				d.shutdown()
				return
			}
		case name := <-d.restarts:
			d.startIfCurrent(name)
		}
	}
}

func (d *daemon) applyConfig(cfg supervisor.Config) {
	desired := map[string]supervisor.Service{}
	for _, svc := range cfg.Services {
		desired[svc.Name] = svc
	}

	for name, state := range d.services {
		svc, keep := desired[name]
		if !keep {
			warn("stopping removed service: " + name)
			d.stopState(state)
			delete(d.services, name)
			continue
		}
		if sameService(state.spec, svc) {
			continue
		}
		warn("restarting changed service: " + name)
		d.stopState(state)
		d.waitStateStopped(state, stopTimeout(state.spec))
		delete(d.services, name)
	}

	for _, svc := range cfg.Services {
		if _, ok := d.services[svc.Name]; ok {
			continue
		}
		state := &serviceState{spec: svc}
		d.services[svc.Name] = state
		d.startState(state)
	}
}

func (d *daemon) startIfCurrent(name string) {
	if d.stopping {
		return
	}
	state := d.services[name]
	if state == nil || state.pid != 0 {
		return
	}
	d.startState(state)
}

func (d *daemon) startState(state *serviceState) {
	if d.stopping || len(state.spec.Argv) == 0 {
		return
	}
	cmd := exec.Command(state.spec.Argv[0], state.spec.Argv[1:]...)
	cmd.Dir = state.spec.Cwd
	cmd.Env = mergeEnv(os.Environ(), state.spec.Env)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		warn("start failed for " + state.spec.Name + ": " + err.Error())
		if state.spec.Restart != supervisor.RestartNever {
			d.scheduleRestart(state)
		}
		return
	}
	state.pid = cmd.Process.Pid
	state.desiredStop = false
	state.startedAt = time.Now()
	d.byPID[state.pid] = state
	warn("started service " + state.spec.Name + " pid=" + strconv.Itoa(state.pid))
	if err := cmd.Process.Release(); err != nil {
		warn("release failed for " + state.spec.Name + ": " + err.Error())
	}
}

func (d *daemon) stopState(state *serviceState) {
	if state == nil || state.pid == 0 || state.desiredStop {
		return
	}
	state.desiredStop = true
	if err := signalProcessGroup(state.pid, syscall.SIGTERM); err != nil {
		warn("term failed for " + state.spec.Name + ": " + err.Error())
	}
}

func (d *daemon) killState(state *serviceState) {
	if state == nil || state.pid == 0 {
		return
	}
	if err := signalProcessGroup(state.pid, syscall.SIGKILL); err != nil {
		warn("kill failed for " + state.spec.Name + ": " + err.Error())
	}
}

func signalProcessGroup(pid int, sig syscall.Signal) error {
	if err := syscall.Kill(-pid, sig); err == nil {
		return nil
	} else if errors.Is(err, syscall.ESRCH) {
		return nil
	}
	return syscall.Kill(pid, sig)
}

func (d *daemon) reapExitedChildren() {
	for {
		var status syscall.WaitStatus
		pid, err := syscall.Wait4(-1, &status, syscall.WNOHANG, nil)
		if pid > 0 {
			d.reapPID(pid, status)
			continue
		}
		if pid == 0 || errors.Is(err, syscall.ECHILD) {
			return
		}
		if errors.Is(err, syscall.EINTR) {
			continue
		}
		warn("reap error: " + err.Error())
		return
	}
}

func (d *daemon) reapPID(pid int, status syscall.WaitStatus) {
	state := d.byPID[pid]
	delete(d.byPID, pid)
	if state == nil {
		warn("reaped child pid=" + strconv.Itoa(pid) + " status=" + exitStatusText(status))
		return
	}
	state.pid = 0
	warn("service " + state.spec.Name + " exited status=" + exitStatusText(status))
	if state.desiredStop || d.stopping || d.services[state.spec.Name] != state {
		return
	}
	if shouldRestart(state.spec.Restart, status) {
		if time.Since(state.startedAt) >= stableRuntime {
			state.restartFailures = 0
		} else {
			state.restartFailures++
		}
		d.scheduleRestart(state)
	}
}

func shouldRestart(policy supervisor.RestartPolicy, status syscall.WaitStatus) bool {
	switch policy {
	case supervisor.RestartAlways:
		return true
	case supervisor.RestartOnFailure:
		return !status.Exited() || status.ExitStatus() != 0
	default:
		return false
	}
}

func (d *daemon) scheduleRestart(state *serviceState) {
	if d.stopping || state == nil || d.services[state.spec.Name] != state {
		return
	}
	delay := restartBackoff(state.restartFailures)
	warn("scheduling restart for " + state.spec.Name + " in " + delay.String())
	name := state.spec.Name
	time.AfterFunc(delay, func() {
		select {
		case d.restarts <- name:
		default:
			warn("restart queue full; dropping restart for " + name)
		}
	})
}

func restartBackoff(failures int) time.Duration {
	if failures <= 1 {
		return restartDelay
	}
	delay := restartDelay
	for i := 1; i < failures; i++ {
		delay *= 2
		if delay >= maxRestartDelay {
			return maxRestartDelay
		}
	}
	return delay
}

func sameService(a, b supervisor.Service) bool {
	if a.Name != b.Name || a.Cwd != b.Cwd || a.Restart != b.Restart || a.StopTimeoutMs != b.StopTimeoutMs {
		return false
	}
	if len(a.Argv) != len(b.Argv) || len(a.Env) != len(b.Env) {
		return false
	}
	for i := range a.Argv {
		if a.Argv[i] != b.Argv[i] {
			return false
		}
	}
	for i := range a.Env {
		if a.Env[i] != b.Env[i] {
			return false
		}
	}
	return true
}

func mergeEnv(base []string, overrides []string) []string {
	out := append([]string(nil), base...)
	positions := make(map[string]int, len(out)+len(overrides))
	for i, entry := range out {
		if key, ok := envKey(entry); ok {
			positions[key] = i
		}
	}
	for _, entry := range overrides {
		key, ok := envKey(entry)
		if !ok {
			continue
		}
		if pos, exists := positions[key]; exists {
			out[pos] = entry
			continue
		}
		positions[key] = len(out)
		out = append(out, entry)
	}
	return out
}

func envKey(entry string) (string, bool) {
	idx := strings.IndexByte(entry, '=')
	if idx <= 0 {
		return "", false
	}
	return entry[:idx], true
}

func (d *daemon) shutdown() {
	d.stopping = true
	for _, state := range d.runningStates() {
		d.stopState(state)
	}
	deadline := time.Now().Add(d.maxStopTimeout())
	for len(d.byPID) > 0 && time.Now().Before(deadline) {
		d.reapExitedChildren()
		if len(d.byPID) == 0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	for _, state := range d.runningStates() {
		d.killState(state)
	}
	killDeadline := time.Now().Add(time.Second)
	for len(d.byPID) > 0 && time.Now().Before(killDeadline) {
		d.reapExitedChildren()
		if len(d.byPID) == 0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	d.reapExitedChildren()
}

func (d *daemon) waitStateStopped(state *serviceState, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for state.pid != 0 && time.Now().Before(deadline) {
		d.reapExitedChildren()
		if state.pid == 0 {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	if state.pid != 0 {
		d.killState(state)
	}
	killDeadline := time.Now().Add(time.Second)
	for state.pid != 0 && time.Now().Before(killDeadline) {
		d.reapExitedChildren()
		if state.pid == 0 {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func (d *daemon) runningStates() []*serviceState {
	out := make([]*serviceState, 0, len(d.byPID))
	for _, state := range d.byPID {
		out = append(out, state)
	}
	return out
}

func (d *daemon) maxStopTimeout() time.Duration {
	max := defaultStopTimeout
	for _, state := range d.runningStates() {
		if timeout := stopTimeout(state.spec); timeout > max {
			max = timeout
		}
	}
	return max
}

func stopTimeout(svc supervisor.Service) time.Duration {
	if svc.StopTimeoutMs == 0 {
		return defaultStopTimeout
	}
	return time.Duration(svc.StopTimeoutMs) * time.Millisecond
}

func exitStatusText(status syscall.WaitStatus) string {
	if status.Exited() {
		return "exit=" + strconv.Itoa(status.ExitStatus())
	}
	if status.Signaled() {
		return "signal=" + status.Signal().String()
	}
	return strconv.Itoa(int(status))
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
	configPath = supervisor.ConfigPath(configPath, os.Getenv)
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

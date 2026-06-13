package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/bartdeboer/go-supervisor/initcfg"
)

func TestSupervisordMainHelper(t *testing.T) {
	if os.Getenv("SUPERVISORD_TEST_MAIN") != "1" {
		return
	}
	os.Args = []string{"supervisord", "--config", os.Getenv("SUPERVISORD_TEST_CONFIG")}
	main()
	os.Exit(0)
}

func TestMergeEnvAllowsServiceOverrides(t *testing.T) {
	got := mergeEnv([]string{"PATH=/bin", "A=base"}, []string{"A=service", "B=service"})
	if valueOf(got, "A") != "service" {
		t.Fatalf("A was not overridden: %v", got)
	}
	if valueOf(got, "PATH") != "/bin" {
		t.Fatalf("PATH changed unexpectedly: %v", got)
	}
	if valueOf(got, "B") != "service" {
		t.Fatalf("B was not added: %v", got)
	}
}

func TestRestartBackoffCaps(t *testing.T) {
	cases := []struct {
		failures int
		want     time.Duration
	}{
		{0, time.Second},
		{1, time.Second},
		{2, 2 * time.Second},
		{3, 4 * time.Second},
		{99, maxRestartDelay},
	}
	for _, tc := range cases {
		if got := restartBackoff(tc.failures); got != tc.want {
			t.Fatalf("restartBackoff(%d)=%s want %s", tc.failures, got, tc.want)
		}
	}
}

func TestDaemonStartsServiceAgainAfterRestart(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "supervisord.config.bin")
	writeConfig(t, configPath, initcfg.Service{
		Name:    "bootmark",
		Cwd:     dir,
		Argv:    []string{"sh", "-c", "echo boot >> boot.log; sleep 30"},
		Restart: initcfg.RestartNever,
	})

	for i := 0; i < 2; i++ {
		cmd := startDaemon(t, configPath)
		waitForLineCount(t, filepath.Join(dir, "boot.log"), i+1, 3*time.Second)
		stopDaemon(t, cmd)
	}
}

func TestDaemonRestartsOnFailureButNotSuccess(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "supervisord.config.bin")
	writeConfig(t, configPath, initcfg.Service{
		Name:    "failer",
		Cwd:     dir,
		Argv:    []string{"sh", "-c", "echo fail >> fail.log; exit 7"},
		Restart: initcfg.RestartOnFailure,
	})
	cmd := startDaemon(t, configPath)
	waitForLineCount(t, filepath.Join(dir, "fail.log"), 2, 5*time.Second)
	stopDaemon(t, cmd)

	configPath = filepath.Join(dir, "supervisord-success.config.bin")
	writeConfig(t, configPath, initcfg.Service{
		Name:    "success",
		Cwd:     dir,
		Argv:    []string{"sh", "-c", "echo success >> success.log; exit 0"},
		Restart: initcfg.RestartOnFailure,
	})
	cmd = startDaemon(t, configPath)
	waitForLineCount(t, filepath.Join(dir, "success.log"), 1, 3*time.Second)
	time.Sleep(1200 * time.Millisecond)
	if got := lineCount(filepath.Join(dir, "success.log")); got != 1 {
		t.Fatalf("exit-0 service restarted unexpectedly; lines=%d", got)
	}
	stopDaemon(t, cmd)
}

func TestDaemonReloadAddMalformedThenRemove(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "supervisord.config.bin")
	writeConfig(t, configPath)
	cmd := startDaemon(t, configPath)
	time.Sleep(100 * time.Millisecond)

	writeConfig(t, configPath, initcfg.Service{
		Name:    "ticker",
		Cwd:     dir,
		Argv:    []string{"sh", "-c", "while true; do echo tick >> ticks.log; sleep 0.1; done"},
		Restart: initcfg.RestartAlways,
	})
	signalDaemon(t, cmd, syscall.SIGHUP)
	waitForLineCount(t, filepath.Join(dir, "ticks.log"), 2, 3*time.Second)

	if err := os.WriteFile(configPath, []byte("bad"), 0o644); err != nil {
		t.Fatal(err)
	}
	signalDaemon(t, cmd, syscall.SIGHUP)
	before := lineCount(filepath.Join(dir, "ticks.log"))
	waitForLineCount(t, filepath.Join(dir, "ticks.log"), before+2, 3*time.Second)

	writeConfig(t, configPath)
	signalDaemon(t, cmd, syscall.SIGHUP)
	stoppedAt := waitForStableLineCount(t, filepath.Join(dir, "ticks.log"), 700*time.Millisecond, 3*time.Second)
	if stoppedAt == 0 {
		t.Fatalf("ticker never wrote ticks before stop")
	}
	stopDaemon(t, cmd)
}

func TestDaemonShutdownKillsProcessGroup(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "supervisord.config.bin")
	writeConfig(t, configPath, initcfg.Service{
		Name:    "group",
		Cwd:     dir,
		Argv:    []string{"sh", "-c", "sleep 60 & echo $! > grandchild.pid; wait"},
		Restart: initcfg.RestartNever,
	})
	cmd := startDaemon(t, configPath)
	pidPath := filepath.Join(dir, "grandchild.pid")
	waitForFile(t, pidPath, 3*time.Second)
	data, err := os.ReadFile(pidPath)
	if err != nil {
		t.Fatal(err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		t.Fatal(err)
	}
	stopDaemon(t, cmd)
	waitForPIDGone(t, pid, 3*time.Second)
}

func valueOf(env []string, key string) string {
	prefix := key + "="
	for _, entry := range env {
		if strings.HasPrefix(entry, prefix) {
			return strings.TrimPrefix(entry, prefix)
		}
	}
	return ""
}

func writeConfig(t *testing.T, path string, services ...initcfg.Service) {
	t.Helper()
	if err := initcfg.WriteConfigFile(path, services); err != nil {
		t.Fatal(err)
	}
}

func startDaemon(t *testing.T, configPath string) *exec.Cmd {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=TestSupervisordMainHelper")
	cmd.Env = append(os.Environ(), "SUPERVISORD_TEST_MAIN=1", "SUPERVISORD_TEST_CONFIG="+configPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if cmd.ProcessState == nil {
			_ = cmd.Process.Signal(syscall.SIGTERM)
			_, _ = cmd.Process.Wait()
		}
	})
	return cmd
}

func signalDaemon(t *testing.T, cmd *exec.Cmd, sig syscall.Signal) {
	t.Helper()
	if err := cmd.Process.Signal(sig); err != nil {
		t.Fatal(err)
	}
}

func stopDaemon(t *testing.T, cmd *exec.Cmd) {
	t.Helper()
	if cmd.ProcessState != nil {
		return
	}
	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil && !errors.Is(err, os.ErrProcessDone) {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatal("supervisord did not stop")
	}
}

func waitForFile(t *testing.T, path string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", path)
}

func waitForLineCount(t *testing.T, path string, want int, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if got := lineCount(path); got >= want {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s to have %d lines; got %d", path, want, lineCount(path))
}

func waitForStableLineCount(t *testing.T, path string, stableFor time.Duration, timeout time.Duration) int {
	t.Helper()
	deadline := time.Now().Add(timeout)
	last := lineCount(path)
	stableSince := time.Now()
	for time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
		got := lineCount(path)
		if got != last {
			last = got
			stableSince = time.Now()
			continue
		}
		if time.Since(stableSince) >= stableFor {
			return got
		}
	}
	t.Fatalf("timed out waiting for stable line count in %s", path)
	return 0
}

func lineCount(path string) int {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	if len(data) == 0 {
		return 0
	}
	return strings.Count(string(data), "\n")
}

func waitForPIDGone(t *testing.T, pid int, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if err := syscall.Kill(pid, 0); errors.Is(err, syscall.ESRCH) {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("pid still exists: %d", pid)
}

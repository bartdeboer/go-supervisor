package main

import (
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"time"
)

const defaultRestartDelay = time.Second

type RestartPolicy string

const (
	RestartNever    RestartPolicy = "never"
	RestartComplete RestartPolicy = "complete"
	RestartError    RestartPolicy = "error"
	RestartCode     RestartPolicy = "code"
)

type Config struct {
	Binary       string
	Args         []string
	Stdin        io.Reader
	Stdout       io.Writer
	Stderr       io.Writer
	Restart      RestartPolicy
	RestartCode  int
	RestartDelay time.Duration
	Logger       *log.Logger
}

type ExitError struct {
	Code int
	Err  error
}

func (err *ExitError) Error() string {
	if err == nil {
		return "<nil>"
	}
	return fmt.Sprintf("process exited: %v", err.Err)
}

func (err *ExitError) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.Err
}

type processExit struct {
	Code int
	Err  error
}

func Run(cfg Config) error {
	cfg = cfg.withDefaults()
	if err := cfg.Validate(); err != nil {
		return err
	}

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, handledSignals()...)
	defer signal.Stop(sigs)

	for {
		cmd, err := startProcess(cfg)
		if err != nil {
			return fmt.Errorf("start process: %w", err)
		}

		waitDone := make(chan error, 1)
		go func() {
			waitDone <- cmd.Wait()
		}()

		select {
		case sig := <-sigs:
			switch {
			case isShutdownSignal(sig):
				cfg.Logger.Printf("Received %s: shutting down", sig)
				if err := stopProcess(cmd); err != nil {
					return fmt.Errorf("stop child process: %w", err)
				}
				<-waitDone
				return nil
			case isRestartSignal(sig):
				cfg.Logger.Printf("Received %s: restarting child process", sig)
				if err := stopProcess(cmd); err != nil {
					return fmt.Errorf("stop child process for restart: %w", err)
				}
				time.Sleep(cfg.RestartDelay)
				<-waitDone
			default:
				cfg.Logger.Printf("Received %s: ignoring unsupported signal", sig)
			}
		case err := <-waitDone:
			exit := classifyProcessExit(err)
			if cfg.shouldRestart(exit) {
				cfg.Logger.Printf("Child exited with code %d: restarting per policy %q", exit.Code, cfg.Restart)
				time.Sleep(cfg.RestartDelay)
				continue
			}
			return exit.toError()
		}
	}
}

func (cfg Config) withDefaults() Config {
	if cfg.Stdin == nil {
		cfg.Stdin = os.Stdin
	}
	if cfg.Stdout == nil {
		cfg.Stdout = os.Stdout
	}
	if cfg.Stderr == nil {
		cfg.Stderr = os.Stderr
	}
	if cfg.Restart == "" {
		cfg.Restart = RestartComplete
	}
	if cfg.RestartDelay <= 0 {
		cfg.RestartDelay = defaultRestartDelay
	}
	if cfg.Logger == nil {
		cfg.Logger = log.New(cfg.Stderr, "", log.LstdFlags)
	}
	return cfg
}

func (cfg Config) Validate() error {
	if cfg.Binary == "" {
		return errors.New("usage: supervisor [--restart=complete|error|never|code] [--restart-code=<n>] <binary> [args...]")
	}

	switch cfg.Restart {
	case RestartNever, RestartComplete, RestartError:
		if cfg.RestartCode != 0 {
			return errors.New("--restart-code is only valid with --restart=code")
		}
	case RestartCode:
	default:
		return fmt.Errorf("invalid restart policy %q", cfg.Restart)
	}

	return nil
}

func startProcess(cfg Config) (*exec.Cmd, error) {
	cmd := exec.Command(cfg.Binary, cfg.Args...)
	cmd.Stdin = cfg.Stdin
	cmd.Stdout = cfg.Stdout
	cmd.Stderr = cfg.Stderr
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return cmd, nil
}

func (cfg Config) shouldRestart(exit processExit) bool {
	switch cfg.Restart {
	case RestartComplete:
		return exit.Code == 0
	case RestartError:
		return exit.Code != 0
	case RestartCode:
		return exit.Code == cfg.RestartCode
	default:
		return false
	}
}

func classifyProcessExit(err error) processExit {
	if err == nil {
		return processExit{Code: 0}
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return processExit{
			Code: exitErr.ExitCode(),
			Err:  err,
		}
	}

	return processExit{
		Code: -1,
		Err:  fmt.Errorf("wait for process: %w", err),
	}
}

func (exit processExit) toError() error {
	if exit.Err == nil {
		return nil
	}
	return &ExitError{
		Code: exit.Code,
		Err:  exit.Err,
	}
}

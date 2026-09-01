package main

import (
	"bytes"
	"errors"
	"os/exec"
	"testing"
	"time"
)

func TestConfigWithDefaults(t *testing.T) {
	stderr := &bytes.Buffer{}

	cfg := (Config{
		Binary: "echo",
		Stderr: stderr,
	}).withDefaults()

	if cfg.Stdin == nil {
		t.Fatal("expected stdin default")
	}
	if cfg.Stdout == nil {
		t.Fatal("expected stdout default")
	}
	if cfg.Stderr != stderr {
		t.Fatal("expected stderr to be preserved")
	}
	if cfg.Restart != RestartComplete {
		t.Fatalf("expected restart policy %q, got %q", RestartComplete, cfg.Restart)
	}
	if cfg.Logger == nil {
		t.Fatal("expected logger default")
	}
	if cfg.RestartDelay != time.Second {
		t.Fatalf("expected restart delay %s, got %s", time.Second, cfg.RestartDelay)
	}
}

func TestConfigValidateRequiresBinary(t *testing.T) {
	err := (Config{}).Validate()
	if err == nil {
		t.Fatal("expected usage error")
	}
	if got, want := err.Error(), "usage: supervisor [--restart=complete|error|never|code] [--restart-code=<n>] <binary> [args...]"; got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestConfigValidateRejectsRestartCodeWithoutCodePolicy(t *testing.T) {
	err := (Config{
		Binary:      "echo",
		Restart:     RestartNever,
		RestartCode: 7,
	}).Validate()
	if err == nil {
		t.Fatal("expected validation error")
	}
	if got, want := err.Error(), "--restart-code is only valid with --restart=code"; got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestConfigValidateRejectsUnknownRestartPolicy(t *testing.T) {
	err := (Config{
		Binary:  "echo",
		Restart: RestartPolicy("maybe"),
	}).Validate()
	if err == nil {
		t.Fatal("expected validation error")
	}
	if got, want := err.Error(), "invalid restart policy \"maybe\""; got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestClassifyProcessExitNil(t *testing.T) {
	exit := classifyProcessExit(nil)
	if exit.Code != 0 {
		t.Fatalf("expected exit code 0, got %d", exit.Code)
	}
	if err := exit.toError(); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestExitErrorWrapsExecExitError(t *testing.T) {
	exit := classifyProcessExit(&exec.ExitError{})

	err := exit.toError()

	var exitErr *ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected ExitError, got %T", err)
	}
	if exitErr.Code != -1 {
		t.Fatalf("expected exit code -1, got %d", exitErr.Code)
	}
}

func TestShouldRestart(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
		exit processExit
		want bool
	}{
		{
			name: "never",
			cfg:  Config{Restart: RestartNever},
			exit: processExit{Code: 0},
			want: false,
		},
		{
			name: "complete on zero",
			cfg:  Config{Restart: RestartComplete},
			exit: processExit{Code: 0},
			want: true,
		},
		{
			name: "complete on error",
			cfg:  Config{Restart: RestartComplete},
			exit: processExit{Code: 3},
			want: false,
		},
		{
			name: "error on non-zero",
			cfg:  Config{Restart: RestartError},
			exit: processExit{Code: 3},
			want: true,
		},
		{
			name: "error on signal",
			cfg:  Config{Restart: RestartError},
			exit: processExit{Code: -1},
			want: true,
		},
		{
			name: "code exact match",
			cfg:  Config{Restart: RestartCode, RestartCode: 7},
			exit: processExit{Code: 7},
			want: true,
		},
		{
			name: "code mismatch",
			cfg:  Config{Restart: RestartCode, RestartCode: 7},
			exit: processExit{Code: 8},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.cfg.shouldRestart(tt.exit); got != tt.want {
				t.Fatalf("expected %v, got %v", tt.want, got)
			}
		})
	}
}

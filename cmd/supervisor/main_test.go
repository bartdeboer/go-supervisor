package main

import (
	"testing"
)

func TestParseConfigDefaults(t *testing.T) {
	cfg, err := parseConfig([]string{"echo", "hello"})
	if err != nil {
		t.Fatalf("parse config: %v", err)
	}

	if cfg.Binary != "echo" {
		t.Fatalf("expected binary echo, got %q", cfg.Binary)
	}
	if len(cfg.Args) != 1 || cfg.Args[0] != "hello" {
		t.Fatalf("unexpected args: %#v", cfg.Args)
	}
	if cfg.Restart != RestartComplete {
		t.Fatalf("expected restart policy %q, got %q", RestartComplete, cfg.Restart)
	}
}

func TestParseConfigRestartCode(t *testing.T) {
	cfg, err := parseConfig([]string{"--restart=code", "--restart-code=7", "echo"})
	if err != nil {
		t.Fatalf("parse config: %v", err)
	}

	if cfg.Restart != RestartCode {
		t.Fatalf("expected restart policy %q, got %q", RestartCode, cfg.Restart)
	}
	if cfg.RestartCode != 7 {
		t.Fatalf("expected restart code 7, got %d", cfg.RestartCode)
	}
}

func TestParseConfigRejectsInvalidCombination(t *testing.T) {
	_, err := parseConfig([]string{"--restart=error", "--restart-code=7", "echo"})
	if err == nil {
		t.Fatal("expected error")
	}
	if got, want := err.Error(), "--restart-code is only valid with --restart=code"; got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

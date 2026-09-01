package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestMainUsesCanonicalCommandName(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if code := runMain([]string{"service", "enable", "help"}, &stdout, &stderr, func(string) string { return "" }); code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "supervisorctl service enable") {
		t.Fatalf("help = %q, want supervisorctl command", stdout.String())
	}
}

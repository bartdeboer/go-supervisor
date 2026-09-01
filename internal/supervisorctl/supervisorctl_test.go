package supervisorctl

import (
	"bytes"
	"strings"
	"testing"
)

func TestMainUsesInvokedCommandName(t *testing.T) {
	for _, command := range []string{"supervisorctl", "supervisor"} {
		t.Run(command, func(t *testing.T) {
			var stdout bytes.Buffer
			var stderr bytes.Buffer
			if code := Main(command, []string{"service", "enable", "help"}, &stdout, &stderr, func(string) string { return "" }); code != 0 {
				t.Fatalf("exit code = %d, stderr = %q", code, stderr.String())
			}
			if !strings.Contains(stdout.String(), command+" service enable") {
				t.Fatalf("help = %q, want command %q", stdout.String(), command)
			}
		})
	}
}

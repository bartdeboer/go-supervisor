package supervisor

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestSupervisordDependencyClosureStaysTiny(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test source")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "list", "-deps", "-json", "./cmd/supervisord")
	cmd.Dir = filepath.Dir(file)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}

	allowed := map[string]bool{
		"github.com/bartdeboer/go-supervisor":                 false,
		"github.com/bartdeboer/go-supervisor/cmd/supervisord": false,
		"github.com/bartdeboer/go-tape/tape":                  false,
	}
	decoder := json.NewDecoder(stdout)
	for {
		var pkg struct {
			ImportPath string
			Standard   bool
		}
		if err := decoder.Decode(&pkg); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			t.Fatalf("decode go list: %v", err)
		}
		if pkg.Standard {
			continue
		}
		if _, ok := allowed[pkg.ImportPath]; !ok {
			t.Errorf("supervisord depends on unexpected package %s", pkg.ImportPath)
			continue
		}
		allowed[pkg.ImportPath] = true
	}
	if err := cmd.Wait(); err != nil {
		t.Fatalf("go list: %v", err)
	}
	for pkg, seen := range allowed {
		if !seen {
			t.Errorf("supervisord dependency missing from closure: %s", pkg)
		}
	}
}

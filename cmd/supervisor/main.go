package main

import (
	"os"

	"github.com/bartdeboer/go-supervisor/internal/supervisorctl"
)

// supervisor remains a compatibility name for the supervisord control CLI.
func main() {
	if code := supervisorctl.Main("supervisor", os.Args[1:], os.Stdout, os.Stderr, os.Getenv); code != 0 {
		os.Exit(code)
	}
}

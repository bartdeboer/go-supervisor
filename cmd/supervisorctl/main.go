package main

import (
	"os"

	"github.com/bartdeboer/go-supervisor/internal/supervisorctl"
)

func main() {
	if code := supervisorctl.Main("supervisorctl", os.Args[1:], os.Stdout, os.Stderr, os.Getenv); code != 0 {
		os.Exit(code)
	}
}

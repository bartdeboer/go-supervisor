package main

import (
	"fmt"
	"os"

	"github.com/bartdeboer/go-supervisor/initcfg"
)

const defaultConfigPath = "/home/agent/state/supervisord.config.bin"

func main() {
	configPath, park, fallback, err := parseArgs(os.Args[1:])
	if err != nil {
		fatal(err)
	}
	cfg, err := initcfg.ReadConfigFile(configPath)
	if err != nil {
		fatal(err)
	}
	fmt.Fprintf(os.Stderr, "supervisord: loaded %d services\n", len(cfg.Services))
	if park {
		fmt.Fprintln(os.Stderr, "supervisord: park mode requested")
	}
	if len(fallback) > 0 {
		fmt.Fprintf(os.Stderr, "supervisord: fallback command configured: %v\n", fallback)
	}
	// Runtime supervision intentionally follows in the next slice.
}

func parseArgs(args []string) (configPath string, park bool, fallback []string, err error) {
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--config":
			i++
			if i >= len(args) || args[i] == "" {
				return "", false, nil, fmt.Errorf("missing --config value")
			}
			configPath = args[i]
		case "--park":
			park = true
		case "--":
			fallback = append([]string(nil), args[i+1:]...)
			i = len(args)
		case "--help", "-h":
			return "", false, nil, errHelp{}
		default:
			return "", false, nil, fmt.Errorf("unknown argument: %s", args[i])
		}
	}
	if configPath == "" {
		configPath = defaultConfigPath
	}
	return configPath, park, fallback, nil
}

type errHelp struct{}

func (errHelp) Error() string { return usage() }

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	fmt.Fprintln(os.Stderr, usage())
	os.Exit(2)
}

func usage() string {
	return "usage: supervisord [--config <path>] [--park] [-- <fallback> [args...]]"
}

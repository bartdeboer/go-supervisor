package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
)

func main() {
	cfg, err := parseConfig(os.Args[1:])
	if err != nil {
		log.Fatal(err)
	}

	if err := Run(cfg); err != nil {
		var exitErr *ExitError
		if errors.As(err, &exitErr) {
			log.Print(err)
			os.Exit(exitErr.Code)
		}
		log.Fatal(err)
	}
}

func parseConfig(args []string) (Config, error) {
	fs := flag.NewFlagSet("supervisor", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	restart := fs.String("restart", string(RestartComplete), "restart policy: complete, error, never, or code")
	restartCode := fs.Int("restart-code", 0, "exit code that triggers restart when --restart=code")

	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: supervisor [--restart=complete|error|never|code] [--restart-code=<n>] <binary> [args...]")
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		return Config{}, err
	}

	rest := fs.Args()
	cfg := Config{
		Binary:      firstArg(rest),
		Args:        remainingArgs(rest),
		Restart:     RestartPolicy(*restart),
		RestartCode: *restartCode,
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func firstArg(args []string) string {
	if len(args) == 0 {
		return ""
	}
	return args[0]
}

func remainingArgs(args []string) []string {
	if len(args) < 2 {
		return nil
	}
	return args[1:]
}

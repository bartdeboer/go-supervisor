package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/bartdeboer/go-supervisor/initcfg"
)

const defaultConfigPath = "/home/agent/state/supervisord.config.bin"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		fmt.Fprintln(os.Stderr, usage())
		os.Exit(2)
	}
}

func run(args []string) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		fmt.Println(usage())
		return nil
	}
	configPath := getenv("SUPERVISORD_CONFIG", defaultConfigPath)
	switch args[0] {
	case "service":
		return runService(configPath, args[1:])
	case "status":
		return listServices(configPath)
	case "reload":
		return reloadSupervisord()
	default:
		return fmt.Errorf("unknown command: %s", args[0])
	}
}

func runService(configPath string, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("missing service command")
	}
	switch args[0] {
	case "list":
		return listServices(configPath)
	case "enable":
		return enableService(configPath, args[1:])
	case "remove":
		if len(args) != 2 || strings.TrimSpace(args[1]) == "" {
			return fmt.Errorf("usage: supervisor service remove <name>")
		}
		return removeService(configPath, args[1])
	default:
		return fmt.Errorf("unknown service command: %s", args[0])
	}
}

func listServices(configPath string) error {
	cfg, err := readConfigIfPresent(configPath)
	if err != nil {
		return err
	}
	if len(cfg.Services) == 0 {
		fmt.Println("no services configured")
		return nil
	}
	for _, svc := range cfg.Services {
		line := fmt.Sprintf("%s restart=%s", svc.Name, restartName(svc.Restart))
		if svc.Cwd != "" {
			line += " cwd=" + svc.Cwd
		}
		if svc.StopTimeoutMs != 0 {
			line += fmt.Sprintf(" stop_timeout_ms=%d", svc.StopTimeoutMs)
		}
		line += " -- " + strings.Join(svc.Argv, " ")
		fmt.Println(line)
	}
	return nil
}

func enableService(configPath string, args []string) error {
	fs := flag.NewFlagSet("supervisor service enable", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	name := fs.String("name", "", "service name; defaults to basename of executable")
	cwd := fs.String("cwd", "", "working directory")
	restart := fs.String("restart", "never", "restart policy: never, on-failure, always")
	stopTimeout := fs.Uint("stop-timeout-ms", 0, "graceful stop timeout in milliseconds")
	var env envFlags
	fs.Var(&env, "env", "environment entry KEY=VALUE; may be repeated")
	if err := fs.Parse(args); err != nil {
		return err
	}
	argv := fs.Args()
	if len(argv) > 0 && argv[0] == "--" {
		argv = argv[1:]
	}
	if len(argv) == 0 {
		return fmt.Errorf("missing service argv; use: supervisor service enable [flags] -- <command> [args...]")
	}
	policy, err := parseRestart(*restart)
	if err != nil {
		return err
	}
	serviceName := strings.TrimSpace(*name)
	if serviceName == "" {
		serviceName = filepath.Base(argv[0])
	}
	svc := initcfg.Service{
		Name:          serviceName,
		Cwd:           strings.TrimSpace(*cwd),
		Argv:          append([]string(nil), argv...),
		Env:           env,
		Restart:       policy,
		StopTimeoutMs: uint32(*stopTimeout),
	}
	if err := initcfg.ValidateService(svc); err != nil {
		return err
	}
	cfg, err := readConfigIfPresent(configPath)
	if err != nil {
		return err
	}
	cfg.Services = upsertService(cfg.Services, svc)
	if err := writeConfig(configPath, cfg); err != nil {
		return err
	}
	fmt.Printf("enabled service: %s\n", svc.Name)
	return reloadSupervisord()
}

func removeService(configPath string, name string) error {
	cfg, err := readConfigIfPresent(configPath)
	if err != nil {
		return err
	}
	name = strings.TrimSpace(name)
	out := cfg.Services[:0]
	removed := false
	for _, svc := range cfg.Services {
		if svc.Name == name {
			removed = true
			continue
		}
		out = append(out, svc)
	}
	if !removed {
		return fmt.Errorf("service not found: %s", name)
	}
	cfg.Services = append([]initcfg.Service(nil), out...)
	if err := writeConfig(configPath, cfg); err != nil {
		return err
	}
	fmt.Printf("removed service: %s\n", name)
	return reloadSupervisord()
}

func readConfigIfPresent(path string) (initcfg.Config, error) {
	cfg, err := initcfg.ReadConfigFile(path)
	if err == nil {
		return cfg, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return initcfg.Config{}, nil
	}
	return initcfg.Config{}, err
}

func writeConfig(path string, cfg initcfg.Config) error {
	return initcfg.WriteConfigFile(path, cfg.Services)
}

func upsertService(services []initcfg.Service, svc initcfg.Service) []initcfg.Service {
	out := append([]initcfg.Service(nil), services...)
	for i := range out {
		if out[i].Name == svc.Name {
			out[i] = svc
			return out
		}
	}
	return append(out, svc)
}

func reloadSupervisord() error {
	// The PID file and SIGHUP handoff land with the supervisord runtime loop.
	return nil
}

type envFlags []string

func (e *envFlags) String() string { return strings.Join(*e, ",") }
func (e *envFlags) Set(value string) error {
	value = strings.TrimSpace(value)
	if value == "" || !strings.Contains(value, "=") || strings.HasPrefix(value, "=") {
		return fmt.Errorf("env expects KEY=VALUE")
	}
	*e = append(*e, value)
	return nil
}

func parseRestart(value string) (initcfg.RestartPolicy, error) {
	switch strings.TrimSpace(value) {
	case "", "never":
		return initcfg.RestartNever, nil
	case "on-failure":
		return initcfg.RestartOnFailure, nil
	case "always":
		return initcfg.RestartAlways, nil
	default:
		return initcfg.RestartNever, fmt.Errorf("unknown restart policy: %s", value)
	}
}

func restartName(policy initcfg.RestartPolicy) string {
	switch policy {
	case initcfg.RestartNever:
		return "never"
	case initcfg.RestartOnFailure:
		return "on-failure"
	case initcfg.RestartAlways:
		return "always"
	default:
		return fmt.Sprintf("unknown(%d)", policy)
	}
}

func getenv(key string, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func usage() string {
	return `usage:
  supervisor service list
  supervisor service enable [--name <name>] [--cwd <dir>] [--restart never|on-failure|always] [--env KEY=VALUE] -- <command> [args...]
  supervisor service remove <name>
  supervisor status
  supervisor reload

environment:
  SUPERVISORD_CONFIG  config path (default /home/agent/state/supervisord.config.bin)`
}

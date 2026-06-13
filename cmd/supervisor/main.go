package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/bartdeboer/go-clir"
	supervisor "github.com/bartdeboer/go-supervisor"
	"github.com/bartdeboer/go-supervisor/internal/defaults"
)

type app struct {
	configPath string
	out        io.Writer
	err        io.Writer
}

func main() {
	a := app{
		configPath: defaults.ConfigPathFrom("", os.Getenv),
		out:        os.Stdout,
		err:        os.Stderr,
	}
	if err := run(context.Background(), a, os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		fmt.Fprintln(os.Stderr)
		_ = newRouter(a).FPrintHelp(context.Background(), os.Stderr, nil)
		os.Exit(2)
	}
}

func run(ctx context.Context, a app, args []string) error {
	r := newRouter(a)
	if len(args) == 0 || clir.IsHelpRequest(args) {
		return r.FPrintHelp(ctx, a.out, clir.StripHelpToken(args))
	}
	return r.Run(ctx, args)
}

func newRouter(a app) *clir.Router {
	r := clir.New()
	r.Routes(func(b *clir.Builder) {
		b.Describe("", "supervisor edits the durable supervisord service config.\n\nEnvironment:\n  SUPERVISORD_CONFIG  config path (default /home/agent/state/supervisord.config.bin)")
		b.Route("service", func(b *clir.Builder) {
			b.Describe("", "Service config commands.")
			b.Handle("list", "List configured services", func(req *clir.Request) error {
				if len(req.Extra) != 0 {
					return unexpectedArgs(req.Extra)
				}
				return listServices(a)
			})
			b.Handle("enable", "Enable or replace one service", func(req *clir.Request) error {
				return enableService(a, req.Extra)
			})
			b.Describe("enable", "Enable or replace one service.\n\nUsage:\n  supervisor service enable [--name <name>] [--cwd <dir>] [--restart never|on-failure|always] [--stop-timeout-ms <ms>] [--env KEY=VALUE] -- <command> [args...]\n\nChanges are written to the durable config. Run `supervisor reload` to apply them to the running supervisord.")
			b.Handle("remove <name>", "Remove one configured service", func(req *clir.Request) error {
				if len(req.Extra) != 0 {
					return unexpectedArgs(req.Extra)
				}
				return removeService(a, req.Params["name"])
			})
			b.Describe("remove <name>", "Remove one configured service from the durable config.\n\nUsage:\n  supervisor service remove <name>\n\nRun `supervisor reload` to apply the removal to the running supervisord.")
		})
		b.Handle("reload", "Signal running supervisord to reload config", func(req *clir.Request) error {
			if len(req.Extra) != 0 {
				return unexpectedArgs(req.Extra)
			}
			return reloadSupervisord(a)
		})
		b.Describe("reload", "Signal PID 1 supervisord to reload its durable config.\n\nUsage:\n  supervisor reload")
	})
	return r
}

func unexpectedArgs(args []string) error {
	return fmt.Errorf("unexpected arguments: %s", strings.Join(args, " "))
}

func listServices(a app) error {
	cfg, err := readConfigIfPresent(a.configPath)
	if err != nil {
		return err
	}
	if len(cfg.Services) == 0 {
		fmt.Fprintln(a.out, "no services configured")
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
		fmt.Fprintln(a.out, line)
	}
	return nil
}

func enableService(a app, args []string) error {
	fs := flag.NewFlagSet("supervisor service enable", flag.ContinueOnError)
	fs.SetOutput(a.err)
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
	svc := supervisor.Service{
		Name:          serviceName,
		Cwd:           strings.TrimSpace(*cwd),
		Argv:          append([]string(nil), argv...),
		Env:           env,
		Restart:       policy,
		StopTimeoutMs: uint32(*stopTimeout),
	}
	if err := supervisor.ValidateService(svc); err != nil {
		return err
	}
	cfg, err := readConfigIfPresent(a.configPath)
	if err != nil {
		return err
	}
	cfg.Services = upsertService(cfg.Services, svc)
	if err := writeConfig(a.configPath, cfg); err != nil {
		return err
	}
	fmt.Fprintf(a.out, "enabled service: %s\n", svc.Name)
	fmt.Fprintln(a.out, "run `supervisor reload` to apply")
	return nil
}

func removeService(a app, name string) error {
	cfg, err := readConfigIfPresent(a.configPath)
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
	cfg.Services = append([]supervisor.Service(nil), out...)
	if err := writeConfig(a.configPath, cfg); err != nil {
		return err
	}
	fmt.Fprintf(a.out, "removed service: %s\n", name)
	fmt.Fprintln(a.out, "run `supervisor reload` to apply")
	return nil
}

func readConfigIfPresent(path string) (supervisor.Config, error) {
	cfg, err := supervisor.ReadConfigFile(path)
	if err == nil {
		return cfg, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return supervisor.Config{}, nil
	}
	return supervisor.Config{}, err
}

func writeConfig(path string, cfg supervisor.Config) error {
	return supervisor.WriteConfigFile(path, cfg.Services)
}

func upsertService(services []supervisor.Service, svc supervisor.Service) []supervisor.Service {
	out := append([]supervisor.Service(nil), services...)
	for i := range out {
		if out[i].Name == svc.Name {
			out[i] = svc
			return out
		}
	}
	return append(out, svc)
}

func reloadSupervisord(a app) error {
	if !pidOneIsSupervisord() {
		return fmt.Errorf("pid 1 is not supervisord; cannot reload")
	}
	if err := syscall.Kill(1, syscall.SIGHUP); err != nil {
		return err
	}
	fmt.Fprintln(a.out, "reload signaled")
	return nil
}

func pidOneIsSupervisord() bool {
	data, err := os.ReadFile("/proc/1/comm")
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(data)) == "supervisord"
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

func parseRestart(value string) (supervisor.RestartPolicy, error) {
	switch strings.TrimSpace(value) {
	case "", "never":
		return supervisor.RestartNever, nil
	case "on-failure":
		return supervisor.RestartOnFailure, nil
	case "always":
		return supervisor.RestartAlways, nil
	default:
		return supervisor.RestartNever, fmt.Errorf("unknown restart policy: %s", value)
	}
}

func restartName(policy supervisor.RestartPolicy) string {
	switch policy {
	case supervisor.RestartNever:
		return "never"
	case supervisor.RestartOnFailure:
		return "on-failure"
	case supervisor.RestartAlways:
		return "always"
	default:
		return fmt.Sprintf("unknown(%d)", policy)
	}
}

package initcfg

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

func Validate(cfg Config) error {
	if len(cfg.Services) > MaxServices {
		return fmt.Errorf("%w: %d > %d", ErrTooManyServices, len(cfg.Services), MaxServices)
	}
	seen := make(map[string]struct{}, len(cfg.Services))
	for i, svc := range cfg.Services {
		if err := ValidateService(svc); err != nil {
			return fmt.Errorf("service %d: %w", i, err)
		}
		if _, ok := seen[svc.Name]; ok {
			return fmt.Errorf("%w: duplicate service name %q", ErrInvalidService, svc.Name)
		}
		seen[svc.Name] = struct{}{}
	}
	return nil
}

func ValidateService(svc Service) error {
	if strings.TrimSpace(svc.Name) == "" {
		return fmt.Errorf("%w: missing name", ErrInvalidService)
	}
	if err := validateText("name", svc.Name, MaxNameLen); err != nil {
		return err
	}
	if svc.Cwd != "" {
		if err := validateText("cwd", svc.Cwd, MaxCwdLen); err != nil {
			return err
		}
	}
	if len(svc.Argv) == 0 {
		return fmt.Errorf("%w: missing argv", ErrInvalidService)
	}
	if len(svc.Argv) > MaxArgv {
		return fmt.Errorf("%w: argv count %d > %d", ErrInvalidService, len(svc.Argv), MaxArgv)
	}
	for i, arg := range svc.Argv {
		if err := validateText(fmt.Sprintf("argv[%d]", i), arg, MaxStringLen); err != nil {
			return err
		}
		if arg == "" && i == 0 {
			return fmt.Errorf("%w: empty argv[0]", ErrInvalidService)
		}
	}
	if len(svc.Env) > MaxEnv {
		return fmt.Errorf("%w: env count %d > %d", ErrInvalidService, len(svc.Env), MaxEnv)
	}
	for i, env := range svc.Env {
		if err := validateText(fmt.Sprintf("env[%d]", i), env, MaxStringLen); err != nil {
			return err
		}
		if !strings.Contains(env, "=") || strings.HasPrefix(env, "=") {
			return fmt.Errorf("%w: malformed env[%d]", ErrInvalidEnv, i)
		}
	}
	if !svc.Restart.Valid() {
		return fmt.Errorf("%w: invalid restart policy %d", ErrInvalidService, svc.Restart)
	}
	return nil
}

func validateText(field string, value string, maxLen int) error {
	if len(value) > maxLen {
		return fmt.Errorf("%w: %s too long", ErrInvalidString, field)
	}
	if !utf8.ValidString(value) {
		return fmt.Errorf("%w: %s is not valid UTF-8", ErrInvalidString, field)
	}
	if strings.IndexByte(value, 0) >= 0 {
		return fmt.Errorf("%w: %s contains NUL", ErrInvalidString, field)
	}
	return nil
}

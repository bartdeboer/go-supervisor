package initcfg

import (
	"strconv"
	"strings"
	"unicode/utf8"
)

func Validate(cfg Config) error {
	if len(cfg.Services) > MaxServices {
		return detail(ErrTooManyServices, strconv.Itoa(len(cfg.Services))+" > "+strconv.Itoa(MaxServices))
	}
	seen := make(map[string]struct{}, len(cfg.Services))
	for i, svc := range cfg.Services {
		if err := ValidateService(svc); err != nil {
			return detail(err, "service "+strconv.Itoa(i))
		}
		if _, ok := seen[svc.Name]; ok {
			return detail(ErrInvalidService, "duplicate service name "+svc.Name)
		}
		seen[svc.Name] = struct{}{}
	}
	return nil
}

func ValidateService(svc Service) error {
	if strings.TrimSpace(svc.Name) == "" {
		return detail(ErrInvalidService, "missing name")
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
		return detail(ErrInvalidService, "missing argv")
	}
	if len(svc.Argv) > MaxArgv {
		return detail(ErrInvalidService, "argv count "+strconv.Itoa(len(svc.Argv))+" > "+strconv.Itoa(MaxArgv))
	}
	for i, arg := range svc.Argv {
		if err := validateText(indexedField("argv", i), arg, MaxStringLen); err != nil {
			return err
		}
		if arg == "" && i == 0 {
			return detail(ErrInvalidService, "empty argv[0]")
		}
	}
	if len(svc.Env) > MaxEnv {
		return detail(ErrInvalidService, "env count "+strconv.Itoa(len(svc.Env))+" > "+strconv.Itoa(MaxEnv))
	}
	for i, env := range svc.Env {
		if err := validateText(indexedField("env", i), env, MaxStringLen); err != nil {
			return err
		}
		if !strings.Contains(env, "=") || strings.HasPrefix(env, "=") {
			return detail(ErrInvalidEnv, "malformed env["+strconv.Itoa(i)+"]")
		}
	}
	if !svc.Restart.Valid() {
		return detail(ErrInvalidService, "invalid restart policy "+strconv.Itoa(int(svc.Restart)))
	}
	return nil
}

func validateText(field string, value string, maxLen int) error {
	if len(value) > maxLen {
		return detail(ErrInvalidString, field+" too long")
	}
	if !utf8.ValidString(value) {
		return detail(ErrInvalidString, field+" is not valid UTF-8")
	}
	if strings.IndexByte(value, 0) >= 0 {
		return detail(ErrInvalidString, field+" contains NUL")
	}
	return nil
}

func indexedField(name string, index int) string {
	return name + "[" + strconv.Itoa(index) + "]"
}

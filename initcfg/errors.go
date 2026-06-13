package initcfg

import "errors"

var (
	ErrConfigTooLarge     = errors.New("initcfg: config too large")
	ErrInvalidMagic       = errors.New("initcfg: invalid magic")
	ErrUnsupportedVersion = errors.New("initcfg: unsupported version")
	ErrUnsupportedFlags   = errors.New("initcfg: unsupported flags")
	ErrTooManyServices    = errors.New("initcfg: too many services")
	ErrInvalidService     = errors.New("initcfg: invalid service")
	ErrInvalidString      = errors.New("initcfg: invalid string")
	ErrInvalidEnv         = errors.New("initcfg: invalid env")
)

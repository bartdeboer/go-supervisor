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

type detailError struct {
	err    error
	detail string
}

func (e detailError) Error() string { return e.err.Error() + ": " + e.detail }
func (e detailError) Unwrap() error { return e.err }

func detail(err error, text string) error {
	return detailError{err: err, detail: text}
}

package initcfg

const (
	Magic   = "CTGINIT\x00"
	Version = uint16(1)
	Flags   = uint16(0)
)

const (
	MaxConfigSize = 256 * 1024
	MaxServices   = 64
	MaxStringLen  = 4096
	MaxArgv       = 128
	MaxEnv        = 256
	MaxNameLen    = 128
	MaxCwdLen     = 4096
)

type Config struct {
	Services []Service
}

type Service struct {
	Name          string
	Cwd           string
	Argv          []string
	Env           []string
	Restart       RestartPolicy
	StopTimeoutMs uint32
}

type RestartPolicy uint8

const (
	RestartNever RestartPolicy = iota
	RestartOnFailure
	RestartAlways
)

func (p RestartPolicy) Valid() bool {
	switch p {
	case RestartNever, RestartOnFailure, RestartAlways:
		return true
	default:
		return false
	}
}

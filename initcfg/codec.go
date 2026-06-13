package initcfg

import (
	"bytes"
	"fmt"
	"os"

	"github.com/bartdeboer/go-tape/tape"
)

const (
	fieldName          = uint8(0x01)
	fieldCwd           = uint8(0x02)
	fieldArgv          = uint8(0x03)
	fieldEnv           = uint8(0x04)
	fieldRestart       = uint8(0x05)
	fieldStopTimeoutMs = uint8(0x06)
)

func WriteConfigFile(path string, services []Service) error {
	data, err := Encode(Config{Services: services})
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func ReadConfigFile(path string) ([]Service, error) {
	cfg, err := DecodeFile(path)
	if err != nil {
		return nil, err
	}
	return cfg.Services, nil
}

func Encode(cfg Config) ([]byte, error) {
	if err := Validate(cfg); err != nil {
		return nil, err
	}
	w := tape.NewWriterCap(estimateConfigSize(cfg))
	_ = w.WriteRaw([]byte(Magic))
	_ = w.WriteU16(Version)
	_ = w.WriteU16(Flags)
	_ = w.WriteU16(uint16(len(cfg.Services)))
	for _, svc := range cfg.Services {
		body, err := encodeService(svc)
		if err != nil {
			return nil, err
		}
		if len(body) > MaxServiceRecordSize {
			return nil, fmt.Errorf("%w: %s", ErrRecordTooLarge, svc.Name)
		}
		_ = w.WriteU32(uint32(len(body)))
		_ = w.WriteRaw(body)
	}
	data, err := w.Bytes()
	if err != nil {
		return nil, err
	}
	if len(data) > MaxConfigSize {
		return nil, fmt.Errorf("%w: %d > %d", ErrConfigTooLarge, len(data), MaxConfigSize)
	}
	return data, nil
}

func DecodeFile(path string) (Config, error) {
	info, err := os.Stat(path)
	if err != nil {
		return Config{}, err
	}
	if info.Size() > MaxConfigSize {
		return Config{}, fmt.Errorf("%w: %d > %d", ErrConfigTooLarge, info.Size(), MaxConfigSize)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	return Decode(data)
}

func Decode(data []byte) (Config, error) {
	if len(data) > MaxConfigSize {
		return Config{}, fmt.Errorf("%w: %d > %d", ErrConfigTooLarge, len(data), MaxConfigSize)
	}
	r := tape.NewReader(data)
	magic, err := r.ReadRaw(len(Magic))
	if err != nil {
		return Config{}, err
	}
	if !bytes.Equal(magic, []byte(Magic)) {
		return Config{}, ErrInvalidMagic
	}
	version, err := r.ReadU16()
	if err != nil {
		return Config{}, err
	}
	if version != Version {
		return Config{}, fmt.Errorf("%w: %d", ErrUnsupportedVersion, version)
	}
	flags, err := r.ReadU16()
	if err != nil {
		return Config{}, err
	}
	if flags != Flags {
		return Config{}, fmt.Errorf("%w: %d", ErrUnsupportedFlags, flags)
	}
	serviceCount, err := r.ReadU16()
	if err != nil {
		return Config{}, err
	}
	if int(serviceCount) > MaxServices {
		return Config{}, fmt.Errorf("%w: %d > %d", ErrTooManyServices, serviceCount, MaxServices)
	}
	cfg := Config{Services: make([]Service, 0, int(serviceCount))}
	for i := 0; i < int(serviceCount); i++ {
		n, err := r.ReadU32()
		if err != nil {
			return Config{}, err
		}
		if n > MaxServiceRecordSize {
			return Config{}, fmt.Errorf("%w: %d > %d", ErrRecordTooLarge, n, MaxServiceRecordSize)
		}
		body, err := r.ReadRaw(int(n))
		if err != nil {
			return Config{}, err
		}
		svc, err := decodeService(body)
		if err != nil {
			return Config{}, fmt.Errorf("service %d: %w", i, err)
		}
		cfg.Services = append(cfg.Services, svc)
	}
	if err := r.RequireDone(); err != nil {
		return Config{}, err
	}
	if err := Validate(cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func encodeService(svc Service) ([]byte, error) {
	w := tape.NewWriter()
	if err := writeStringField(w, fieldName, svc.Name, MaxNameLen); err != nil {
		return nil, err
	}
	if svc.Cwd != "" {
		if err := writeStringField(w, fieldCwd, svc.Cwd, MaxCwdLen); err != nil {
			return nil, err
		}
	}
	if err := writeStringListField(w, fieldArgv, svc.Argv, MaxArgv, MaxStringLen); err != nil {
		return nil, err
	}
	if len(svc.Env) > 0 {
		if err := writeStringListField(w, fieldEnv, svc.Env, MaxEnv, MaxStringLen); err != nil {
			return nil, err
		}
	}
	if err := writeU8Field(w, fieldRestart, uint8(svc.Restart)); err != nil {
		return nil, err
	}
	if svc.StopTimeoutMs != 0 {
		if err := writeU32Field(w, fieldStopTimeoutMs, svc.StopTimeoutMs); err != nil {
			return nil, err
		}
	}
	return w.Bytes()
}

func decodeService(body []byte) (Service, error) {
	r := tape.NewReader(body)
	var svc Service
	seen := map[uint8]bool{}
	for r.Remaining() > 0 {
		tag, err := r.ReadU8()
		if err != nil {
			return svc, err
		}
		if seen[tag] {
			return svc, fmt.Errorf("%w: tag 0x%02x", ErrDuplicateField, tag)
		}
		seen[tag] = true
		n, err := r.ReadU32()
		if err != nil {
			return svc, err
		}
		payload, err := r.ReadRaw(int(n))
		if err != nil {
			return svc, err
		}
		fr := tape.NewReader(payload)
		switch tag {
		case fieldName:
			v, err := fr.ReadString(MaxNameLen)
			if err != nil {
				return svc, err
			}
			svc.Name = v
		case fieldCwd:
			v, err := fr.ReadString(MaxCwdLen)
			if err != nil {
				return svc, err
			}
			svc.Cwd = v
		case fieldArgv:
			v, err := fr.ReadStringList(MaxArgv, MaxStringLen)
			if err != nil {
				return svc, err
			}
			svc.Argv = v
		case fieldEnv:
			v, err := fr.ReadStringList(MaxEnv, MaxStringLen)
			if err != nil {
				return svc, err
			}
			svc.Env = v
		case fieldRestart:
			v, err := fr.ReadU8()
			if err != nil {
				return svc, err
			}
			svc.Restart = RestartPolicy(v)
		case fieldStopTimeoutMs:
			v, err := fr.ReadU32()
			if err != nil {
				return svc, err
			}
			svc.StopTimeoutMs = v
		default:
			continue
		}
		if err := fr.RequireDone(); err != nil {
			return svc, err
		}
	}
	if !seen[fieldName] {
		return svc, fmt.Errorf("%w: name", ErrMissingField)
	}
	if !seen[fieldArgv] {
		return svc, fmt.Errorf("%w: argv", ErrMissingField)
	}
	if !seen[fieldRestart] {
		svc.Restart = RestartNever
	}
	return svc, ValidateService(svc)
}

func writeField(w *tape.Writer, tag uint8, payload []byte) error {
	if len(payload) > MaxServiceRecordSize {
		return ErrRecordTooLarge
	}
	if err := w.WriteU8(tag); err != nil {
		return err
	}
	if err := w.WriteU32(uint32(len(payload))); err != nil {
		return err
	}
	return w.WriteRaw(payload)
}

func writeStringField(w *tape.Writer, tag uint8, value string, maxLen int) error {
	pw := tape.NewWriter()
	if err := pw.WriteString(value, maxLen); err != nil {
		return err
	}
	payload, err := pw.Bytes()
	if err != nil {
		return err
	}
	return writeField(w, tag, payload)
}

func writeStringListField(w *tape.Writer, tag uint8, values []string, maxCount int, maxItemLen int) error {
	pw := tape.NewWriter()
	if err := pw.WriteStringList(values, maxCount, maxItemLen); err != nil {
		return err
	}
	payload, err := pw.Bytes()
	if err != nil {
		return err
	}
	return writeField(w, tag, payload)
}

func writeU8Field(w *tape.Writer, tag uint8, value uint8) error {
	pw := tape.NewWriter()
	_ = pw.WriteU8(value)
	payload, err := pw.Bytes()
	if err != nil {
		return err
	}
	return writeField(w, tag, payload)
}

func writeU32Field(w *tape.Writer, tag uint8, value uint32) error {
	pw := tape.NewWriter()
	_ = pw.WriteU32(value)
	payload, err := pw.Bytes()
	if err != nil {
		return err
	}
	return writeField(w, tag, payload)
}

func estimateConfigSize(cfg Config) int {
	return 14 + len(cfg.Services)*64
}

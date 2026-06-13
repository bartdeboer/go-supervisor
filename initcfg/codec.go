package initcfg

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"

	"github.com/bartdeboer/go-tape/tape"
)

func WriteConfigFile(path string, services []Service) error {
	data, err := Encode(Config{Services: services})
	if err != nil {
		return err
	}
	return writeFileAtomic(path, data, 0o644)
}

func ReadConfigFile(path string) (Config, error) {
	return DecodeFile(path)
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
		if err := encodeService(w, svc); err != nil {
			return nil, err
		}
	}
	data, err := w.Bytes()
	if err != nil {
		return nil, err
	}
	if len(data) > MaxConfigSize {
		return nil, detail(ErrConfigTooLarge, strconv.Itoa(len(data))+" > "+strconv.Itoa(MaxConfigSize))
	}
	return data, nil
}

func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

func DecodeFile(path string) (Config, error) {
	info, err := os.Stat(path)
	if err != nil {
		return Config{}, err
	}
	if info.Size() > MaxConfigSize {
		return Config{}, detail(ErrConfigTooLarge, strconv.FormatInt(info.Size(), 10)+" > "+strconv.Itoa(MaxConfigSize))
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	return Decode(data)
}

func Decode(data []byte) (Config, error) {
	if len(data) > MaxConfigSize {
		return Config{}, detail(ErrConfigTooLarge, strconv.Itoa(len(data))+" > "+strconv.Itoa(MaxConfigSize))
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
		return Config{}, detail(ErrUnsupportedVersion, strconv.Itoa(int(version)))
	}
	flags, err := r.ReadU16()
	if err != nil {
		return Config{}, err
	}
	if flags != Flags {
		return Config{}, detail(ErrUnsupportedFlags, strconv.Itoa(int(flags)))
	}
	serviceCount, err := r.ReadU16()
	if err != nil {
		return Config{}, err
	}
	if int(serviceCount) > MaxServices {
		return Config{}, detail(ErrTooManyServices, strconv.Itoa(int(serviceCount))+" > "+strconv.Itoa(MaxServices))
	}
	cfg := Config{Services: make([]Service, 0, int(serviceCount))}
	for i := 0; i < int(serviceCount); i++ {
		svc, err := decodeService(r)
		if err != nil {
			return Config{}, detail(err, "service "+strconv.Itoa(i))
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

func encodeService(w *tape.Writer, svc Service) error {
	if err := w.WriteString(svc.Name, MaxNameLen); err != nil {
		return err
	}
	if err := w.WriteString(svc.Cwd, MaxCwdLen); err != nil {
		return err
	}
	if err := w.WriteStringList(svc.Argv, MaxArgv, MaxStringLen); err != nil {
		return err
	}
	if err := w.WriteStringList(svc.Env, MaxEnv, MaxStringLen); err != nil {
		return err
	}
	if err := w.WriteU8(uint8(svc.Restart)); err != nil {
		return err
	}
	if err := w.WriteU32(svc.StopTimeoutMs); err != nil {
		return err
	}
	return w.Err()
}

func decodeService(r *tape.Reader) (Service, error) {
	var svc Service
	var err error
	if svc.Name, err = r.ReadString(MaxNameLen); err != nil {
		return svc, err
	}
	if svc.Cwd, err = r.ReadString(MaxCwdLen); err != nil {
		return svc, err
	}
	if svc.Argv, err = r.ReadStringList(MaxArgv, MaxStringLen); err != nil {
		return svc, err
	}
	if svc.Env, err = r.ReadStringList(MaxEnv, MaxStringLen); err != nil {
		return svc, err
	}
	restart, err := r.ReadU8()
	if err != nil {
		return svc, err
	}
	svc.Restart = RestartPolicy(restart)
	if svc.StopTimeoutMs, err = r.ReadU32(); err != nil {
		return svc, err
	}
	return svc, ValidateService(svc)
}

func estimateConfigSize(cfg Config) int {
	return 14 + len(cfg.Services)*64
}

package initcfg

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestEncodeDecodeRoundTrip(t *testing.T) {
	want := Config{Services: []Service{
		{Name: "web", Cwd: "/app", Argv: []string{"/app/server", "--port", "8080"}, Env: []string{"PORT=8080"}, Restart: RestartAlways, StopTimeoutMs: 1500},
		{Name: "worker", Argv: []string{"/app/worker"}, Env: []string{}, Restart: RestartOnFailure},
	}}
	data, err := Encode(want)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v want %#v", got, want)
	}
}

func TestReadWriteConfigFile(t *testing.T) {
	services := []Service{{Name: "web", Argv: []string{"server"}, Env: []string{}, Restart: RestartNever}}
	path := filepath.Join(t.TempDir(), "init.ctg")
	if err := WriteConfigFile(path, services); err != nil {
		t.Fatal(err)
	}
	got, err := ReadConfigFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Services, services) {
		t.Fatalf("got %#v", got)
	}
}

func TestDecodeRejectsInvalidMagic(t *testing.T) {
	data, err := Encode(Config{Services: []Service{{Name: "web", Argv: []string{"server"}}}})
	if err != nil {
		t.Fatal(err)
	}
	copy(data[:len(Magic)], []byte("BADINIT\x00"))
	_, err = Decode(data)
	if !errors.Is(err, ErrInvalidMagic) {
		t.Fatalf("err=%v", err)
	}
}

func TestDecodeRejectsTrailingData(t *testing.T) {
	data, err := Encode(Config{})
	if err != nil {
		t.Fatal(err)
	}
	data = append(data, 1)
	_, err = Decode(data)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestDecodeFileRejectsOversize(t *testing.T) {
	path := filepath.Join(t.TempDir(), "big.ctg")
	if err := os.WriteFile(path, bytes.Repeat([]byte{0}, MaxConfigSize+1), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := DecodeFile(path)
	if !errors.Is(err, ErrConfigTooLarge) {
		t.Fatalf("err=%v", err)
	}
}

func FuzzDecode(f *testing.F) {
	data, err := Encode(Config{Services: []Service{{Name: "web", Argv: []string{"server"}}}})
	if err != nil {
		f.Fatal(err)
	}
	f.Add(data)
	f.Add([]byte{})
	f.Add([]byte(Magic))
	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = Decode(data)
	})
}

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
		{Name: "worker", Argv: []string{"/app/worker"}, Restart: RestartOnFailure},
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
	services := []Service{{Name: "web", Argv: []string{"server"}, Restart: RestartNever}}
	path := filepath.Join(t.TempDir(), "init.ctg")
	if err := WriteConfigFile(path, services); err != nil {
		t.Fatal(err)
	}
	got, err := ReadConfigFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, services) {
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

func TestDecodeSkipsUnknownServiceField(t *testing.T) {
	cfg := Config{Services: []Service{{Name: "web", Argv: []string{"server"}}}}
	data, err := Encode(cfg)
	if err != nil {
		t.Fatal(err)
	}

	// Append an unknown empty field to the first service body and fix its length.
	origLen := uint32(data[14]) | uint32(data[15])<<8 | uint32(data[16])<<16 | uint32(data[17])<<24
	insert := []byte{0x99, 0, 0, 0, 0}
	data = append(data, insert...)
	newLen := origLen + uint32(len(insert))
	data[14], data[15], data[16], data[17] = byte(newLen), byte(newLen>>8), byte(newLen>>16), byte(newLen>>24)
	got, err := Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, cfg) {
		t.Fatalf("got %#v", got)
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

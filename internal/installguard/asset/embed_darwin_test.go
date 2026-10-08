package asset

import (
	"bytes"
	"compress/gzip"
	"debug/pe"
	"os"
	"testing"
)

func TestEmbeddedBridgeMatchesSources(t *testing.T) {
	want, err := SourceHash("../../..")
	if err != nil {
		t.Fatal(err)
	}
	r, err := gzip.NewReader(bytes.NewReader(helper))
	if err != nil {
		t.Fatal(err)
	}
	//nolint:errcheck // read-only in-memory/file reader cleanup cannot affect the result.
	defer r.Close()
	if r.Comment != "source-sha256:"+want {
		t.Fatal("stale Win32 bridge: run go generate ./internal/installguard/asset")
	}
	path, cleanup, err := Extract(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	exe, err := pe.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if exe.Machine != pe.IMAGE_FILE_MACHINE_AMD64 {
		t.Errorf("bridge machine = %x", exe.Machine)
	}
	//nolint:errcheck // read-only in-memory/file reader cleanup cannot affect the result.
	if err := exe.Close(); err != nil {
		t.Error(err)
	}
	cleanup()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("bridge was not removed: %v", err)
	}
}

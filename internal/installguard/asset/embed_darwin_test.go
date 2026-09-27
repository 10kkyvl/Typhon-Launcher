package asset

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"debug/pe"
	"encoding/hex"
	"os"
	"testing"
)

func TestEmbeddedBridgeMatchesSources(t *testing.T) {
	hash := sha256.New()
	for _, path := range []string{"../../../cmd/installguard/main_windows.go", "../guard_windows.go", "../job_windows.go", "../policy.go", "../options_windows.go", "../checklist_wine_windows.go", "../../../go.mod", "../../../go.sum", "generate.go"} {
		source, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		hash.Write(source)
	}
	r, err := gzip.NewReader(bytes.NewReader(helper))
	if err != nil {
		t.Fatal(err)
	}
	//nolint:errcheck // read-only in-memory/file reader cleanup cannot affect the result.
	defer r.Close()
	if r.Comment != "source-sha256:"+hex.EncodeToString(hash.Sum(nil)) {
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

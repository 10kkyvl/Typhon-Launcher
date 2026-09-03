//go:build devmock && !windows

package selfupdate

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func seedReadyArtifact(t *testing.T, configDir, version, name string, content []byte) (installerPath string, art Artifact) {
	t.Helper()
	installerPath, err := ArtifactPath(configDir, version, name)
	if err != nil {
		t.Fatalf("ArtifactPath: %v", err)
	}
	writeTestFile(t, installerPath, content)
	art = Artifact{
		OS: runtime.GOOS, Arch: runtime.GOARCH, Kind: KindInstaller, Name: name,
		URL: "https://example.com/" + name, Size: int64(len(content)), SHA256: sha256Hex(t, content),
	}
	store, err := NewStore(configDir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	if err := store.Save(stored{AvailableVersion: version, Artifact: &art, ReadyPath: installerPath}); err != nil {
		t.Fatalf("seed store: %v", err)
	}
	return installerPath, art
}

func TestApplyReplacesTargetAndChangesDigest(t *testing.T) {
	configDir := testConfigDir(t)
	installDir := t.TempDir()
	target := filepath.Join(installDir, "typhon")
	writeTestFile(t, target, []byte("old launcher bytes"))
	before := sha256Hex(t, []byte("old launcher bytes"))

	newContent := []byte("new launcher bytes, longer than before")
	installerPath, _ := seedReadyArtifact(t, configDir, "2.0.0", "typhon-devmock", newContent)

	if err := Apply(context.Background(), installerPath, installDir, target); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	if string(got) != string(newContent) {
		t.Fatalf("target content = %q, want %q", got, newContent)
	}
	if after := sha256Hex(t, got); after == before {
		t.Fatal("target digest did not change")
	}
	info, err := os.Stat(target)
	if err != nil {
		t.Fatalf("stat target: %v", err)
	}
	if info.Mode().Perm()&0o111 == 0 {
		t.Fatalf("target mode = %v, want executable", info.Mode())
	}

	entries, err := os.ReadDir(installDir)
	if err != nil {
		t.Fatalf("read install dir: %v", err)
	}
	for _, e := range entries {
		if e.Name() != filepath.Base(target) {
			t.Fatalf("install dir still has a staging leftover: %s", e.Name())
		}
	}
}

func TestApplyRejectsInstallerOutsideCache(t *testing.T) {
	testConfigDir(t)
	installDir := t.TempDir()
	target := filepath.Join(installDir, "typhon")
	writeTestFile(t, target, []byte("old launcher"))

	outside := filepath.Join(t.TempDir(), "setup")
	writeTestFile(t, outside, []byte("payload"))

	if err := Apply(context.Background(), outside, installDir, target); !errors.Is(err, errInstallerOutsideCache) {
		t.Fatalf("Apply() error = %v, want errInstallerOutsideCache", err)
	}
	if data, err := os.ReadFile(target); err != nil || string(data) != "old launcher" {
		t.Fatalf("target = %q, %v; want untouched", data, err)
	}
}

func TestApplyRejectsReadyPathMismatch(t *testing.T) {
	configDir := testConfigDir(t)
	installDir := t.TempDir()
	target := filepath.Join(installDir, "typhon")
	writeTestFile(t, target, []byte("old launcher"))

	seedReadyArtifact(t, configDir, "2.0.0", "typhon-devmock", []byte("new launcher"))

	other, err := ArtifactPath(configDir, "2.0.1", "typhon-devmock")
	if err != nil {
		t.Fatalf("ArtifactPath: %v", err)
	}
	writeTestFile(t, other, []byte("new launcher"))

	if err := Apply(context.Background(), other, installDir, target); !errors.Is(err, ErrNotReady) {
		t.Fatalf("Apply() error = %v, want ErrNotReady when the path does not match the recorded ReadyPath", err)
	}
	if data, err := os.ReadFile(target); err != nil || string(data) != "old launcher" {
		t.Fatalf("target = %q, %v; want untouched", data, err)
	}
}

func TestApplyRejectsHashMismatch(t *testing.T) {
	configDir := testConfigDir(t)
	installDir := t.TempDir()
	target := filepath.Join(installDir, "typhon")
	writeTestFile(t, target, []byte("old launcher"))

	content := []byte("new launcher bytes")
	installerPath, _ := seedReadyArtifact(t, configDir, "2.0.0", "typhon-devmock", content)

	tampered := make([]byte, len(content))
	copy(tampered, content)
	tampered[0] ^= 0xff
	writeTestFile(t, installerPath, tampered)

	if err := Apply(context.Background(), installerPath, installDir, target); !errors.Is(err, ErrHashMismatch) {
		t.Fatalf("Apply() error = %v, want ErrHashMismatch", err)
	}
	if data, err := os.ReadFile(target); err != nil || string(data) != "old launcher" {
		t.Fatalf("target = %q, %v; want untouched", data, err)
	}
}

func TestApplyMissingTargetLeavesNothingWritten(t *testing.T) {
	configDir := testConfigDir(t)
	installDir := t.TempDir()
	target := filepath.Join(installDir, "typhon")

	installerPath, _ := seedReadyArtifact(t, configDir, "2.0.0", "typhon-devmock", []byte("new launcher"))

	if err := Apply(context.Background(), installerPath, installDir, target); err == nil {
		t.Fatal("Apply() error = nil, want an error for a missing target file")
	}

	entries, err := os.ReadDir(installDir)
	if err != nil {
		t.Fatalf("read install dir: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("install dir = %v, want empty: nothing should be written when the target is missing", entries)
	}
}

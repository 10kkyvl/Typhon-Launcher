package clientid

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
)

func TestLoadAtEmptyPath(t *testing.T) {
	if _, err := LoadAt(""); err == nil {
		t.Fatal("expected error for empty path")
	}
}

func TestLoadAtCreatesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "installation.json")

	id, err := LoadAt(path)
	if err != nil {
		t.Fatalf("LoadAt: %v", err)
	}
	if _, err := uuid.Parse(id.InstallationID); err != nil {
		t.Fatalf("installation id is not a uuid: %v", err)
	}
	if _, err := uuid.Parse(id.SessionID); err != nil {
		t.Fatalf("session id is not a uuid: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected file to be created: %v", err)
	}
}

func TestLoadAtPersistsInstallationID(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "installation.json")

	first, err := LoadAt(path)
	if err != nil {
		t.Fatalf("first LoadAt: %v", err)
	}
	second, err := LoadAt(path)
	if err != nil {
		t.Fatalf("second LoadAt: %v", err)
	}
	if first.InstallationID != second.InstallationID {
		t.Fatalf("installation id changed across loads: %q != %q", first.InstallationID, second.InstallationID)
	}
}

func TestLoadAtSessionIDDiffersEachCall(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "installation.json")

	first, err := LoadAt(path)
	if err != nil {
		t.Fatalf("first LoadAt: %v", err)
	}
	second, err := LoadAt(path)
	if err != nil {
		t.Fatalf("second LoadAt: %v", err)
	}
	if first.SessionID == second.SessionID {
		t.Fatal("expected a fresh session id on each LoadAt call")
	}
}

func TestLoadAtCorruptJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "installation.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatalf("seed corrupt file: %v", err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read seeded file: %v", err)
	}

	if _, err := LoadAt(path); err == nil {
		t.Fatal("expected error for corrupt json")
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file after failed load: %v", err)
	}
	if string(before) != string(after) {
		t.Fatal("corrupt file was overwritten instead of being left untouched")
	}
}

func TestLoadAtInvalidUUID(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "installation.json")
	if err := os.WriteFile(path, []byte(`{"installationId":"not-a-uuid"}`), 0o600); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	if _, err := LoadAt(path); err == nil {
		t.Fatal("expected error for invalid uuid in file")
	}
}

func TestLoadAtPathIsDirectory(t *testing.T) {
	dir := t.TempDir()
	asDir := filepath.Join(dir, "installation.json")
	if err := os.Mkdir(asDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	if _, err := LoadAt(asDir); err == nil {
		t.Fatal("expected error when path is a directory instead of a file")
	}
}

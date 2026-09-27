//go:build devmock && !windows

package shortcut

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestCreateWritesJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "Game.json")
	link := Link{
		Target:      "/Applications/Game.app",
		Args:        "--fullscreen",
		WorkDir:     "/Applications",
		Icon:        "/Applications/Game.app/icon.png",
		Description: "Play Game",
	}
	if err := Create(path, link); err != nil {
		t.Fatalf("Create() err = %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() err = %v", err)
	}
	var got Link
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("Unmarshal() err = %v", err)
	}
	if got != link {
		t.Fatalf("got %+v, want %+v", got, link)
	}
}

func TestCreateEmptyTarget(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "Game.json")
	if err := Create(path, Link{}); err == nil {
		t.Fatal("Create() err = nil, want error for empty target")
	}
}

func TestCreateMissingDir(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing", "Game.json")
	if err := Create(path, Link{Target: "/bin/true"}); err == nil {
		t.Fatal("Create() err = nil, want error for missing dir")
	}
}

func TestRemoveExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "Game.json")
	if err := Create(path, Link{Target: "/bin/true"}); err != nil {
		t.Fatalf("Create() err = %v", err)
	}
	if err := Remove(path); err != nil {
		t.Fatalf("Remove() err = %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("stat after Remove() err = %v, want not exist", err)
	}
}

func TestDesktopDir(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	if _, err := DesktopDir(); err == nil {
		t.Fatal("DesktopDir() err = nil, want error when Desktop is missing")
	}

	desktop := filepath.Join(tmp, "Desktop")
	if err := os.Mkdir(desktop, 0o700); err != nil {
		t.Fatal(err)
	}
	got, err := DesktopDir()
	if err != nil {
		t.Fatalf("DesktopDir() err = %v", err)
	}
	if got != desktop {
		t.Fatalf("DesktopDir() = %q, want %q", got, desktop)
	}
}

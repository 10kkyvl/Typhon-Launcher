package library

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestMarkerRoundTrip(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "bin", "game.exe")
	game := Game{
		ID:              "abc",
		Title:           "Portal",
		Executable:      exe,
		InstallDir:      dir,
		Version:         "1.2",
		VersionSource:   "release_metadata",
		CanonicalGameID: "cid",
		InstalledAt:     time.Now().Truncate(time.Second),
	}
	if err := WriteMarker(dir, markerFor(game)); err != nil {
		t.Fatalf("write marker: %v", err)
	}

	marker, err := ReadMarker(dir)
	if err != nil {
		t.Fatalf("read marker: %v", err)
	}
	if marker.GameID != game.ID || marker.Title != game.Title || marker.CanonicalGameID != game.CanonicalGameID {
		t.Fatalf("marker = %+v, want identity of %+v", marker, game)
	}
	if marker.Executable != "bin/game.exe" {
		t.Fatalf("executable = %q, want a path relative to the install dir", marker.Executable)
	}
}

func TestMarkerExecutableFollowsRenamedDir(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "Portal")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	game := Game{ID: "abc", Title: "Portal", InstallDir: dir, Executable: filepath.Join(dir, "game.exe")}
	if err := WriteMarker(dir, markerFor(game)); err != nil {
		t.Fatalf("write marker: %v", err)
	}
	renamed := filepath.Join(root, "Portal Renamed")
	if err := os.Rename(dir, renamed); err != nil {
		t.Fatal(err)
	}

	marker, err := ReadMarker(renamed)
	if err != nil {
		t.Fatalf("read marker: %v", err)
	}
	if got, want := marker.MarkerExecutable(renamed), filepath.Join(renamed, "game.exe"); got != want {
		t.Fatalf("executable = %q, want %q", got, want)
	}
	if marker.MarkerExecutable("") != "" {
		t.Fatal("без каталога путь не восстановить")
	}
}

func TestMarkerExecutableRejectsEscapingPath(t *testing.T) {
	marker := Marker{GameID: "abc", Executable: "../outside/game.exe"}
	if got := marker.MarkerExecutable(t.TempDir()); got != "" {
		t.Fatalf("executable = %q, want empty for a path outside the install dir", got)
	}
}

func TestReadMarkerMissing(t *testing.T) {
	if _, err := ReadMarker(t.TempDir()); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("error = %v, want fs.ErrNotExist", err)
	}
}

func TestReadMarkerRejectsBadContent(t *testing.T) {
	cases := []struct {
		name string
		raw  string
	}{
		{"обрезанный json", `{"version":1,"data":{"gameId":`},
		{"мусор", `not json at all`},
		{"нет идентификатора", `{"version":1,"data":{"title":"Portal"}}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, MarkerName), []byte(tc.raw), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := ReadMarker(dir); err == nil {
				t.Fatal("испорченная метка не должна читаться как валидная")
			} else if errors.Is(err, fs.ErrNotExist) {
				t.Fatalf("error = %v, want a parse failure, not a missing file", err)
			}
		})
	}
}

func TestWriteMarkerRejectsEmptyInput(t *testing.T) {
	if err := WriteMarker("", Marker{GameID: "abc"}); err == nil {
		t.Fatal("пустой каталог должен отклоняться")
	}
	if err := WriteMarker(t.TempDir(), Marker{Title: "Portal"}); err == nil {
		t.Fatal("метка без идентификатора должна отклоняться")
	}
}

func TestRegisterInstalledLeavesMarker(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "game.exe")
	if err := os.WriteFile(exe, []byte("stub"), 0o600); err != nil {
		t.Fatal(err)
	}
	s := mustServiceAt(t, filepath.Join(t.TempDir(), "library.json"))
	game, err := s.RegisterInstalled(InstalledGame{Title: "Portal", Executable: exe, InstallDir: dir, CanonicalGameID: "cid"})
	if err != nil {
		t.Fatalf("register installed: %v", err)
	}

	marker, err := ReadMarker(dir)
	if err != nil {
		t.Fatalf("read marker: %v", err)
	}
	if marker.GameID != game.ID || marker.CanonicalGameID != "cid" {
		t.Fatalf("marker = %+v, want identity of %+v", marker, game)
	}
	if game.Source != SourceManaged {
		t.Fatalf("source = %q, want %q", game.Source, SourceManaged)
	}
}

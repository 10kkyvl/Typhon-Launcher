package relocate

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"typhon/internal/library"
)

func externalLibrary(t *testing.T) (*library.Service, string, string) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "Steam Game")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "data.bin"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	games := []library.Game{{
		ID:         "ext",
		Title:      "Steam Game",
		InstallDir: dir,
		Source:     library.SourceExternal,
		External:   &library.External{Provider: library.ProviderSteam, AppID: "42", LaunchURI: "steam://rungameid/42"},
	}}
	raw, err := json.Marshal(games)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "library.json")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	lib, err := library.NewServiceAt(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := lib.ServiceShutdown(); err != nil {
			t.Errorf("library shutdown: %v", err)
		}
	})
	return lib, "ext", dir
}

func TestMoveGameRefusesExternalGame(t *testing.T) {
	lib, id, dir := externalLibrary(t)
	s := newTestService(t, nil, lib, nil, nil, nil)

	if _, err := s.MoveGame(id, filepath.Join(t.TempDir(), "target")); !errors.Is(err, ErrExternalGame) {
		t.Fatalf("err = %v, want ErrExternalGame", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "data.bin")); err != nil {
		t.Fatalf("source touched: %v", err)
	}
	if got := s.List(); len(got) != 0 {
		t.Fatalf("job registered for an external game: %+v", got)
	}
}

func TestMoveLibrarySkipsExternalGames(t *testing.T) {
	lib, id, dir := externalLibrary(t)
	set := newTestSettings(t)
	s := newTestService(t, set, lib, nil, nil, nil)
	normal := addGameAt(t, lib, set.GetSettings().GamesPath, "Plain")

	job, err := s.MoveLibrary(t.TempDir())
	if err != nil {
		t.Fatalf("move library: %v", err)
	}
	if slices.Contains(job.Queue, id) || !slices.Contains(job.Queue, normal.ID) {
		t.Fatalf("queue = %v: external game must be skipped, managed one kept", job.Queue)
	}
	s.wait(job.ID)

	ext, err := lib.Find(id)
	if err != nil {
		t.Fatal(err)
	}
	if ext.InstallDir != dir {
		t.Fatalf("external installDir = %q, want untouched %q", ext.InstallDir, dir)
	}
	if _, err := os.Stat(filepath.Join(dir, "data.bin")); err != nil {
		t.Fatalf("external game files moved: %v", err)
	}
}

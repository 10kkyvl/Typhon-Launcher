package library

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"typhon/internal/emulators"
	"typhon/internal/storage"
)

const crashedHours = int64(3600)

func setPlaytime(t *testing.T, s *Service, id string, seconds int64) {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	game := s.findLocked(id)
	if game == nil {
		t.Fatalf("game %s not found", id)
	}
	game.PlaytimeSeconds = seconds
	if err := s.persist(); err != nil {
		t.Fatal(err)
	}
}

// crashedRemoval leaves the files as a process killed between the two writes of
// a removal leaves them: library.json has the stub, the second file the game.
func crashedRemoval(t *testing.T) (path string, kept, removed Game) {
	t.Helper()
	path = filepath.Join(t.TempDir(), "library.json")
	s := mustServiceAt(t, path)
	kept = emulatedGame(t, s, "kept.iso")
	removed, err := s.AddEmulatedGame(t.Context(), writeFile(t, filepath.Join(t.TempDir(), "gone.iso"), "image"), emulators.PS2, "Gone")
	if err != nil {
		t.Fatal(err)
	}
	setPlaytime(t, s, kept.ID, crashedHours)
	if err := s.RemoveGame(removed.ID); err != nil {
		t.Fatal(err)
	}
	beforeCrash, err := os.ReadFile(emulatedPathFor(path))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.RemoveGame(kept.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.ServiceShutdown(); err != nil {
		t.Fatal(err)
	}
	writeFile(t, emulatedPathFor(path), string(beforeCrash))
	return path, kept, removed
}

func historyOf(games []Game, id string) (hours int64, entries int) {
	for _, game := range games {
		if game.ID == id {
			hours += game.PlaytimeSeconds
			entries++
		}
	}
	return hours, entries
}

func TestCrashInsideRemovalCountsTheHoursOnce(t *testing.T) {
	path, kept, removed := crashedRemoval(t)
	crashed, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	s := mustServiceAt(t, path)

	if hours, entries := historyOf(s.GetHistoryGames(), kept.ID); hours != crashedHours || entries != 1 {
		t.Fatalf("history of the game: %d seconds in %d entries, want %d in 1", hours, entries, crashedHours)
	}
	if _, err := s.Find(kept.ID); err != nil {
		t.Fatalf("the game the crash interrupted must stay: %v", err)
	}
	if _, entries := historyOf(s.GetHistoryGames(), removed.ID); entries != 1 {
		t.Fatalf("stub of a finished removal lost: %d entries", entries)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(crashed) {
		t.Fatal("start-up rewrote library.json")
	}
}

func TestStaleStubLeavesTheFileWithTheNextSave(t *testing.T) {
	path, kept, _ := crashedRemoval(t)
	s := mustServiceAt(t, path)
	if _, err := s.SetFavorite(kept.ID, true); err != nil {
		t.Fatal(err)
	}
	saved, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(saved), kept.ID) {
		t.Fatalf("the stub survived a save: %s", saved)
	}
	if err := s.RemoveGame(kept.ID); err != nil {
		t.Fatal(err)
	}
	if hours, entries := historyOf(s.GetHistoryGames(), kept.ID); hours != crashedHours || entries != 1 {
		t.Fatalf("after a real removal: %d seconds in %d entries, want %d in 1", hours, entries, crashedHours)
	}
}

func TestEmulatedFileThatCannotBeTrustedStopsTheConstructor(t *testing.T) {
	const native = `[{"id":"native","title":"Native","executable":"","installDir":""}]`
	newer := fmt.Sprintf(`{"version":%d,"data":[]}`, emulatedVersion+1)
	denied := func(string, int, map[int]storage.Migration, any) error {
		return fmt.Errorf("read: %w", fs.ErrPermission)
	}
	cases := []struct {
		name     string
		body     string
		load     func(string, int, map[int]storage.Migration, any) error
		mustFind error
	}{
		{name: "no rights", body: `{"version":1,"data":[]}`, load: denied, mustFind: fs.ErrPermission},
		{name: "newer version", body: newer},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if c.load != nil {
				loadEmulatedFile = c.load
				t.Cleanup(func() { loadEmulatedFile = storage.Load })
			}
			path := writeFile(t, filepath.Join(t.TempDir(), "library.json"), native)
			writeFile(t, emulatedPathFor(path), c.body)

			_, err := NewServiceAt(path)
			if err == nil {
				t.Fatal("the constructor must refuse: a save would wipe the games it could not read")
			}
			if c.mustFind != nil && !errors.Is(err, c.mustFind) {
				t.Fatalf("error = %v, want %v", err, c.mustFind)
			}
			got, err := os.ReadFile(emulatedPathFor(path))
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != c.body {
				t.Fatalf("file rewritten: %s", got)
			}
		})
	}
}

func TestFolderInPlaceOfTheEmulatedFileStopsTheConstructor(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	if err := os.Mkdir(emulatedPathFor(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := NewServiceAt(path); err == nil {
		t.Fatal("an unreadable file is not a missing one")
	}
}

package install

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"typhon/internal/library"
	"typhon/internal/uierr"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// journalRig is a service on a config dir prepared the way a killed launcher
// leaves it, started like the next launch.
type journalRig struct {
	dir       string
	s         *Service
	registrar *fakeRegistrar
}

func newJournalRig(t *testing.T, dir string, lib registrar) journalRig {
	t.Helper()
	s := mustServiceAt(t, dir)
	s.settings = newTestSettings(t)
	s.downloads = newFakeDownloads()
	r := journalRig{dir: dir, s: s}
	if fake, ok := lib.(*fakeRegistrar); ok {
		r.registrar = fake
	}
	s.library = lib
	return r
}

func (r journalRig) start(t *testing.T) {
	t.Helper()
	if err := r.s.ServiceStartup(context.Background(), application.ServiceOptions{}); err != nil {
		t.Fatalf("startup: %v", err)
	}
}

// restart is the next launch: ServiceShutdown waits for the startup sweep.
func (r journalRig) restart(t *testing.T) {
	t.Helper()
	r.start(t)
	if err := r.s.ServiceShutdown(); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
}

func writeJournal(t *testing.T, dir string, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "removals.json"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func readJournal(t *testing.T, dir string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, "removals.json"))
	if errors.Is(err, os.ErrNotExist) {
		return ""
	}
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func journalEntry(t *testing.T, path, gameID, installDir string, keep bool) string {
	t.Helper()
	data, err := json.Marshal([]map[string]any{{
		"path": path, "gameId": gameID, "installDir": installDir, "keepInLibrary": keep,
	}})
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func requireJournalHolds(t *testing.T, dir string, paths ...string) {
	t.Helper()
	body := readJournal(t, dir)
	for _, p := range paths {
		encoded, err := json.Marshal(p)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(body, string(encoded)) {
			t.Fatalf("removals.json = %s, want it to hold %s", body, p)
		}
	}
}

func requireJournalClear(t *testing.T, dir string) {
	t.Helper()
	body := strings.TrimSpace(readJournal(t, dir))
	if body != "" && body != "[]" && body != "null" {
		t.Fatalf("removals.json = %s, want no pending entry", body)
	}
}

func putOwnedCard(t *testing.T, registrar *fakeRegistrar, id, dir string) {
	t.Helper()
	registrar.put(library.Game{ID: id, Title: id, InstallDir: dir, Executable: filepath.Join(dir, "Game.exe"), Owned: true, InstallType: string(TypePortable)})
}

func TestRemovalBeforeTheSweepKeepsTheEarlierEntry(t *testing.T) {
	dir := t.TempDir()
	earlier := filepath.Join(t.TempDir(), "Killed"+removingSuffix)
	mkFile(t, filepath.Join(earlier, "leftover.bin"), 64)
	encoded, err := json.Marshal([]string{earlier})
	if err != nil {
		t.Fatal(err)
	}
	writeJournal(t, dir, string(encoded))

	r := newJournalRig(t, dir, &fakeRegistrar{})
	game := gameDir(t, "Game")
	putOwnedCard(t, r.registrar, "g1", game)
	if err := r.s.RemoveGame("g1", RemoveOptions{DeleteFiles: true}); err != nil {
		t.Fatalf("RemoveGame before the sweep: %v", err)
	}
	requireJournalHolds(t, dir, earlier)

	r.restart(t)
	if !missing(t, earlier) {
		t.Fatal("the folder of the killed removal survived the sweep")
	}
	requireJournalClear(t, dir)
}

func TestUnreadableRemovalJournalIsNeverOverwritten(t *testing.T) {
	cases := []struct {
		name    string
		prepare func(t *testing.T, path string)
	}{
		{"corrupt", func(t *testing.T, path string) {
			if err := os.WriteFile(path, []byte(`[{"path": "C:\\Games\\X.removing"`), 0o600); err != nil {
				t.Fatal(err)
			}
		}},
		{"directory in its place", func(t *testing.T, path string) {
			mkFile(t, filepath.Join(path, "keep.txt"), 4)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "removals.json")
			tc.prepare(t, path)
			before := snapshotPath(t, path)

			r := newJournalRig(t, dir, &fakeRegistrar{})
			r.start(t)
			t.Cleanup(func() {
				if err := r.s.ServiceShutdown(); err != nil {
					t.Errorf("shutdown: %v", err)
				}
			})
			game := gameDir(t, "Game")
			putOwnedCard(t, r.registrar, "g1", game)

			err := r.s.RemoveGame("g1", RemoveOptions{DeleteFiles: true})
			if code := uierr.Code(err); code != "install.removals_unreadable" {
				t.Fatalf("RemoveGame with an unreadable journal = %q (%v), want install.removals_unreadable", code, err)
			}
			if missing(t, filepath.Join(game, "Game.exe")) {
				t.Fatal("files deleted although the journal could not be read")
			}
			requireNoRemovalStaging(t, game)
			if card, err := r.registrar.Find("g1"); err != nil || card.Uninstalled {
				t.Fatalf("card = %+v, %v; want it installed", card, err)
			}
			if after := snapshotPath(t, path); after != before {
				t.Fatalf("removals.json changed: %q -> %q", before, after)
			}

			if err := r.s.RemoveGame("g1", RemoveOptions{}); err != nil {
				t.Fatalf("a removal that keeps the files = %v", err)
			}
		})
	}
}

func snapshotPath(t *testing.T, path string) string {
	t.Helper()
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.IsDir() {
		entries, err := os.ReadDir(path)
		if err != nil {
			t.Fatal(err)
		}
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}
		return "dir:" + strings.Join(names, ",")
	}
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestMissingRemovalJournalIsEmpty(t *testing.T) {
	dir := t.TempDir()
	r := newJournalRig(t, dir, &fakeRegistrar{})
	r.start(t)
	t.Cleanup(func() {
		if err := r.s.ServiceShutdown(); err != nil {
			t.Errorf("shutdown: %v", err)
		}
	})
	game := gameDir(t, "Game")
	putOwnedCard(t, r.registrar, "g1", game)
	if err := r.s.RemoveGame("g1", RemoveOptions{DeleteFiles: true}); err != nil {
		t.Fatalf("RemoveGame without a journal: %v", err)
	}
	if !missing(t, game) {
		t.Fatal("game folder still there")
	}
	requireJournalClear(t, dir)
}

// The process dies at each point between the journal entry and the library
// update; the next start ends with the folder gone and the card as asked.
func TestKilledRemovalFinishesWholeAtStartup(t *testing.T) {
	points := []struct {
		name    string
		prepare func(t *testing.T, game, staged string)
	}{
		{"after the rename", func(t *testing.T, game, staged string) {
			if err := os.Rename(game, staged); err != nil {
				t.Fatal(err)
			}
		}},
		{"mid-delete", func(t *testing.T, game, staged string) {
			if err := os.Rename(game, staged); err != nil {
				t.Fatal(err)
			}
			if err := os.RemoveAll(filepath.Join(staged, "data")); err != nil {
				t.Fatal(err)
			}
		}},
		{"after the delete", func(t *testing.T, game, _ string) {
			if err := os.RemoveAll(game); err != nil {
				t.Fatal(err)
			}
		}},
	}
	for _, point := range points {
		for _, keep := range []bool{false, true} {
			name := point.name + "/remove from library"
			if keep {
				name = point.name + "/keep the card"
			}
			t.Run(name, func(t *testing.T) {
				dir := t.TempDir()
				r := newJournalRig(t, dir, &fakeRegistrar{})
				game := gameDir(t, "Game")
				putOwnedCard(t, r.registrar, "g1", game)
				staged := game + removingSuffix
				writeJournal(t, dir, journalEntry(t, staged, "g1", game, keep))
				point.prepare(t, game, staged)

				r.restart(t)

				if !missing(t, staged) || !missing(t, game) {
					t.Fatal("the folder of the killed removal is still on disk")
				}
				card, err := r.registrar.Find("g1")
				switch {
				case keep && (err != nil || !card.Uninstalled):
					t.Fatalf("card = %+v, %v; want it kept and uninstalled", card, err)
				case !keep && !errors.Is(err, errFakeNoGame):
					t.Fatalf("card = %+v, %v; want it removed from the library", card, err)
				}
				requireJournalClear(t, dir)
			})
		}
	}
}

func TestOldRemovalJournalIsStillSwept(t *testing.T) {
	dir := t.TempDir()
	r := newJournalRig(t, dir, &fakeRegistrar{})
	game := gameDir(t, "Game")
	putOwnedCard(t, r.registrar, "g1", game)
	staged := game + removingSuffix
	if err := os.Rename(game, staged); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal([]string{staged})
	if err != nil {
		t.Fatal(err)
	}
	writeJournal(t, dir, string(encoded))

	r.restart(t)

	if !missing(t, staged) {
		t.Fatal("the folder of an old journal entry survived the sweep")
	}
	card, err := r.registrar.Find("g1")
	if err != nil || card.Uninstalled {
		t.Fatalf("card = %+v, %v; an old entry names no game, so the card is left as it was", card, err)
	}
	requireJournalClear(t, dir)
}

func TestSweepLeavesACardThatMovedOn(t *testing.T) {
	dir := t.TempDir()
	r := newJournalRig(t, dir, &fakeRegistrar{})
	old := filepath.Join(t.TempDir(), "Game")
	reinstalled := gameDir(t, "Game")
	putOwnedCard(t, r.registrar, "g1", reinstalled)
	writeJournal(t, dir, journalEntry(t, old+removingSuffix, "g1", old, false))

	r.restart(t)

	if _, err := r.registrar.Find("g1"); err != nil {
		t.Fatalf("a card installed again elsewhere was removed: %v", err)
	}
	if missing(t, filepath.Join(reinstalled, "Game.exe")) {
		t.Fatal("files of the new install deleted")
	}
	requireJournalClear(t, dir)
}

// The game was installed again since the killed removal: the sweep leaves its
// card and the record of the new installation alone.
func TestSweepKeepsTheRecordsOfAGameInstalledAgain(t *testing.T) {
	dir := t.TempDir()
	r := newJournalRig(t, dir, &fakeRegistrar{})
	old := filepath.Join(t.TempDir(), "Game")
	reinstalled := gameDir(t, "Game")
	putOwnedCard(t, r.registrar, "g1", reinstalled)
	record := Installation{ID: "i1", GameID: "g1", Name: "Game", Status: StatusCompleted, Destination: reinstalled}
	if err := r.s.store.save([]Installation{record}); err != nil {
		t.Fatal(err)
	}
	writeJournal(t, dir, journalEntry(t, old+removingSuffix, "g1", old, false))

	r.restart(t)

	after := mustServiceAt(t, dir)
	saved, err := after.store.load()
	if err != nil {
		t.Fatal(err)
	}
	if len(saved) != 1 || saved[0].ID != "i1" {
		t.Fatalf("installations after the sweep = %+v, want the new installation kept", saved)
	}
	requireJournalClear(t, dir)
}

// failingLibrary is a library whose half of the removal fails to save.
type failingLibrary struct {
	*fakeRegistrar
	fail bool
}

var errLibrarySave = errors.New("save library: disk full")

func (f *failingLibrary) RemoveGame(id string) error {
	if f.fail {
		return errLibrarySave
	}
	return f.fakeRegistrar.RemoveGame(id)
}

// An entry stays in the journal after its folder is gone when its drop or its
// library half failed. A later removal of another game from the same folder
// stages under a name of its own: run under the stale entry, a kill would
// settle the other card, or this one the other way.
func TestRemovalNeverStagesUnderAJournaledName(t *testing.T) {
	dir := t.TempDir()
	lib := &failingLibrary{fakeRegistrar: &fakeRegistrar{}, fail: true}
	r := newJournalRig(t, dir, lib)
	game := gameDir(t, "Game")
	lib.put(library.Game{ID: "a", Title: "a", InstallDir: game, Uninstalled: true})
	putOwnedCard(t, lib.fakeRegistrar, "b", game)
	writeJournal(t, dir, journalEntry(t, game+removingSuffix, "a", game, true))

	if err := r.s.RemoveGame("b", RemoveOptions{DeleteFiles: true}); !errors.Is(err, errLibrarySave) {
		t.Fatalf("RemoveGame = %v, want the library failure", err)
	}
	if !missing(t, game) {
		t.Fatal("files still there")
	}
	requireJournalHolds(t, dir, "a", "b")

	lib.fail = false
	next := newJournalRig(t, dir, lib)
	next.restart(t)
	if _, err := lib.Find("b"); !errors.Is(err, errFakeNoGame) {
		t.Fatalf("card b after the next start = %v, want it removed as its removal asked", err)
	}
	card, err := lib.Find("a")
	if err != nil || !card.Uninstalled || card.InstallDir != game {
		t.Fatalf("card a = %+v, %v; want it left as it was", card, err)
	}
	requireJournalClear(t, dir)
}

func TestLibraryHalfOfARemovalIsRetriedAtTheNextStart(t *testing.T) {
	dir := t.TempDir()
	lib := &failingLibrary{fakeRegistrar: &fakeRegistrar{}, fail: true}
	r := newJournalRig(t, dir, lib)
	game := gameDir(t, "Game")
	putOwnedCard(t, lib.fakeRegistrar, "g1", game)
	r.start(t)

	if err := r.s.RemoveGame("g1", RemoveOptions{DeleteFiles: true}); !errors.Is(err, errLibrarySave) {
		t.Fatalf("RemoveGame = %v, want the library failure", err)
	}
	if !missing(t, game) {
		t.Fatal("files still there")
	}
	if err := r.s.ServiceShutdown(); err != nil {
		t.Fatal(err)
	}
	if _, err := lib.Find("g1"); err != nil {
		t.Fatalf("card gone although the library failed: %v", err)
	}

	next := newJournalRig(t, dir, lib)
	next.restart(t)
	if _, err := lib.Find("g1"); err != nil {
		t.Fatalf("card after a start with the library still failing = %v, want it there until the library saves", err)
	}
	requireJournalHolds(t, dir, "g1")

	lib.fail = false
	last := newJournalRig(t, dir, lib)
	last.restart(t)
	if _, err := lib.Find("g1"); !errors.Is(err, errFakeNoGame) {
		t.Fatalf("card after the library recovered = %v, want it removed", err)
	}
	requireJournalClear(t, dir)
}

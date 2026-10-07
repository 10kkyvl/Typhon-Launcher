package library

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"
	"time"
)

const fullGameJSON = `[{
  "id": "g1",
  "title": "Full Game",
  "executable": "C:\\Games\\Full\\game.exe",
  "launchArgs": ["-windowed", "-skip-intro"],
  "requiresSteam": false,
  "installDir": "C:\\Games\\Full",
  "cover": "cover.jpg",
  "version": "1.2.3",
  "versionSource": "release_metadata",
  "versionConfidence": 0.9,
  "sizeBytes": 1234,
  "sizeUnknown": true,
  "lastPlayed": "2026-09-01T10:00:00Z",
  "playtimeSeconds": 3600,
  "installedAt": "2026-08-01T10:00:00Z",
  "sourceDownloadId": "dl-1",
  "releaseId": "rel-1",
  "sourceId": "src-1",
  "distributionId": "dist-1",
  "releaseUploadedAt": "2026-07-01T10:00:00Z",
  "canonicalGameId": "canon-1",
  "repacker": "Rep",
  "releaseVersion": "1.2.3",
  "source": "managed",
  "installType": "installer",
  "owned": true,
  "uninstall": {"key": "K", "command": "C", "quietCommand": "Q", "productCode": "{P}"},
  "uninstallUnknown": true,
  "uninstalled": true,
  "shortcutPath": "C:\\Desktop\\Full.lnk",
  "savesDir": "C:\\Saves\\Full",
  "favorite": true,
  "favoriteAt": "2026-09-02T10:00:00Z",
  "status": "playing",
  "statusAt": "2026-09-03T10:00:00Z"
}]`

func decodeRecords(t *testing.T, data []byte) []map[string]any {
	t.Helper()
	var records []map[string]any
	if err := json.Unmarshal(data, &records); err != nil {
		t.Fatalf("decode %q: %v", data, err)
	}
	return records
}

func TestLibraryFileKeepsItsOnDiskContract(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	if err := os.WriteFile(path, []byte(fullGameJSON), 0o600); err != nil {
		t.Fatal(err)
	}
	s := mustServiceAt(t, path)

	games := s.GetGames()
	if len(games) != 1 {
		t.Fatalf("loaded %d games, want 1", len(games))
	}
	g := games[0]
	if g.ID != "g1" || g.Title != "Full Game" || g.PlaytimeSeconds != 3600 || g.Status != StatusPlaying ||
		!g.Favorite || !g.Uninstalled || g.RequiresSteam == nil || *g.RequiresSteam ||
		g.Uninstall.ProductCode != "{P}" || !slices.Equal(g.LaunchArgs, []string{"-windowed", "-skip-intro"}) {
		t.Fatalf("a field written by an earlier release was not read back: %+v", g)
	}

	s.mu.Lock()
	err := s.persist()
	s.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	want := decodeRecords(t, []byte(fullGameJSON))
	got := decodeRecords(t, readFile(t, path))
	if !reflect.DeepEqual(want, got) {
		t.Fatalf("on-disk schema changed: a state file from an earlier release no longer round-trips\nwant %v\ngot  %v", want, got)
	}
}

func snapshotDir(t *testing.T, dir string) map[string]string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, e := range entries {
		if e.IsDir() {
			out[e.Name()] = "<dir>"
			continue
		}
		out[e.Name()] = string(readFile(t, filepath.Join(dir, e.Name())))
	}
	return out
}

func TestUnreadableStateRefusesToStartAndIsLeftAlone(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T, dir string)
	}{
		{"library file is a directory", func(t *testing.T, dir string) {
			if err := os.Mkdir(filepath.Join(dir, "library.json"), 0o755); err != nil {
				t.Fatal(err)
			}
		}},
		{"library file is empty", func(t *testing.T, dir string) {
			if err := os.WriteFile(filepath.Join(dir, "library.json"), nil, 0o600); err != nil {
				t.Fatal(err)
			}
		}},
		{"library file is cut off mid-record", func(t *testing.T, dir string) {
			if err := os.WriteFile(filepath.Join(dir, "library.json"), []byte(fullGameJSON[:60]), 0o600); err != nil {
				t.Fatal(err)
			}
		}},
		{"excluded list is corrupt", func(t *testing.T, dir string) {
			if err := os.WriteFile(filepath.Join(dir, "library.json"), []byte(fullGameJSON), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "library-excluded.json"), []byte(`{"not":"a list"}`), 0o600); err != nil {
				t.Fatal(err)
			}
		}},
		{"excluded list is a directory", func(t *testing.T, dir string) {
			if err := os.WriteFile(filepath.Join(dir, "library.json"), []byte(fullGameJSON), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(filepath.Join(dir, "library-excluded.json"), 0o755); err != nil {
				t.Fatal(err)
			}
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			tc.setup(t, dir)
			before := snapshotDir(t, dir)

			if s, err := NewServiceAt(filepath.Join(dir, "library.json")); err == nil {
				if shutdownErr := s.ServiceShutdown(); shutdownErr != nil {
					t.Error(shutdownErr)
				}
				t.Fatal("unreadable state produced a working, empty service that will overwrite it")
			}
			if after := snapshotDir(t, dir); !reflect.DeepEqual(before, after) {
				t.Fatalf("a refused start changed the state directory\nbefore %v\nafter  %v", before, after)
			}
		})
	}
}

func TestMissingStateFilesStartEmptyAndCreateThemOnFirstSave(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "library.json")
	s := mustServiceAt(t, path)
	if len(s.GetGames()) != 0 {
		t.Fatalf("games = %+v, want none", s.GetGames())
	}
	if entries, err := os.ReadDir(dir); err != nil || len(entries) != 0 {
		t.Fatalf("starting without state created files: %v %v", entries, err)
	}
	if _, err := s.AddGame(tempGameExe(t), "First"); err != nil {
		t.Fatal(err)
	}
	if got := decodeRecords(t, readFile(t, path)); len(got) != 1 || got[0]["title"] != "First" {
		t.Fatalf("first save wrote %v", got)
	}
}

type saveFixture struct {
	s       *Service
	path    string
	a       Game
	u       Game
	freshEx string
}

type libState struct {
	games    []Game
	archived []Game
	excluded []string
}

func (f *saveFixture) snapshot() libState {
	f.s.mu.Lock()
	defer f.s.mu.Unlock()
	return libState{
		games:    append([]Game(nil), f.s.games...),
		archived: append([]Game(nil), f.s.archived...),
		excluded: append([]string(nil), f.s.excluded...),
	}
}

func newSaveFixture(t *testing.T) *saveFixture {
	t.Helper()
	path := filepath.Join(t.TempDir(), "library.json")
	s := mustServiceAt(t, path)
	exeA := tempGameExe(t)
	a, err := s.RegisterInstalled(InstalledGame{
		Title: "A", Executable: exeA, InstallDir: filepath.Dir(exeA), Version: "1.0",
		SourceID: "src", ReleaseID: "rel", CanonicalGameID: "canon-a",
	})
	if err != nil {
		t.Fatal(err)
	}
	u, err := s.AddGame(tempGameExe(t), "U")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.MarkUninstalled(u.ID); err != nil {
		t.Fatal(err)
	}
	return &saveFixture{s: s, path: path, a: a, u: u, freshEx: tempGameExe(t)}
}

func (f *saveFixture) breakSaving(t *testing.T) {
	t.Helper()
	if err := os.Remove(f.path); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(f.path, 0o755); err != nil {
		t.Fatal(err)
	}
}

func (f *saveFixture) repairSaving(t *testing.T) {
	t.Helper()
	if err := os.Remove(f.path); err != nil {
		t.Fatal(err)
	}
}

func TestMutationsRollBackMemoryWhenTheSaveFails(t *testing.T) {
	uploaded := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name string
		run  func(f *saveFixture) error
	}{
		{"AddGame", func(f *saveFixture) error { _, err := f.s.AddGame(f.freshEx, "New"); return err }},
		{"AddGame reviving an uninstalled card", func(f *saveFixture) error {
			_, err := f.s.AddGame(f.u.Executable, "Back")
			return err
		}},
		{"RegisterInstalled new game", func(f *saveFixture) error {
			_, err := f.s.RegisterInstalled(InstalledGame{Title: "New", Executable: f.freshEx, InstallDir: filepath.Dir(f.freshEx)})
			return err
		}},
		{"RegisterInstalled existing game", func(f *saveFixture) error {
			_, err := f.s.RegisterInstalled(InstalledGame{
				Title: "Renamed", Executable: f.a.Executable, InstallDir: f.a.InstallDir, Version: "9.9", ReleaseID: "other",
			})
			return err
		}},
		{"ApplyInstalledUpdate", func(f *saveFixture) error {
			_, err := f.s.ApplyInstalledUpdate(InstalledUpdate{ID: f.a.ID, Version: "2.0", ReleaseID: "rel-2", SourceID: "src", DistributionID: "d"})
			return err
		}},
		{"BindDistribution", func(f *saveFixture) error {
			_, err := f.s.BindDistribution(f.a.ID, "src", "rel", "dist-1", &uploaded)
			return err
		}},
		{"RemoveGame", func(f *saveFixture) error { return f.s.RemoveGame(f.a.ID) }},
		{"MarkUninstalled", func(f *saveFixture) error { return f.s.MarkUninstalled(f.a.ID) }},
		{"SetFavorite", func(f *saveFixture) error { _, err := f.s.SetFavorite(f.a.ID, true); return err }},
		{"SetStatus", func(f *saveFixture) error { _, err := f.s.SetStatus(f.a.ID, StatusCompleted); return err }},
		{"SetRequiresSteam", func(f *saveFixture) error { _, err := f.s.SetRequiresSteam(f.a.ID, false); return err }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newSaveFixture(t)
			before := f.snapshot()
			f.breakSaving(t)

			if err := tc.run(f); err == nil {
				t.Fatal("the save failed but the operation reported success")
			}
			if after := f.snapshot(); !reflect.DeepEqual(before, after) {
				t.Fatalf("memory kept a change that never reached the disk\nbefore %+v\nafter  %+v", before, after)
			}

			f.repairSaving(t)
			if _, err := f.s.SetStatus(f.a.ID, StatusPlaying); err != nil {
				t.Fatalf("the next save after the disk recovered failed: %v", err)
			}
			reloaded := mustServiceAt(t, f.path).GetGames()
			if len(reloaded) != len(before.games) {
				t.Fatalf("reloaded %d games, want %d: the failed operation was committed by a later save", len(reloaded), len(before.games))
			}
			for _, g := range reloaded {
				if g.ID == f.a.ID {
					if g.Status != StatusPlaying || g.Version != "1.0" || g.Title != "A" || g.Favorite || g.Uninstalled {
						t.Fatalf("the failed operation leaked into a later save: %+v", g)
					}
				}
			}
		})
	}
}

func TestRemovedGamesKeepTheirPlaytimeForTheHistory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	seed := `[
  {"id": "a", "title": "A", "installDir": "", "executable": "", "playtimeSeconds": 50},
  {"id": "a", "title": "A", "playtimeSeconds": 30, "archived": true},
  {"id": "b", "title": "B", "playtimeSeconds": 70, "archived": true, "status": "completed", "statusAt": "2026-09-03T10:00:00Z"}
]`
	if err := os.WriteFile(path, []byte(seed), 0o600); err != nil {
		t.Fatal(err)
	}
	s := mustServiceAt(t, path)

	if games := s.GetGames(); len(games) != 1 || games[0].ID != "a" {
		t.Fatalf("GetGames = %+v, want only the live card", games)
	}
	history := map[string]int64{}
	for _, g := range s.GetHistoryGames() {
		history[g.ID] += g.PlaytimeSeconds
	}
	if !reflect.DeepEqual(history, map[string]int64{"a": 80, "b": 70}) {
		t.Fatalf("history playtime = %v, want a=80 (live+archived) and b=70", history)
	}

	if _, err := s.SetStatus("a", StatusPlaying); err != nil {
		t.Fatal(err)
	}
	onDisk := decodeRecords(t, readFile(t, path))
	if len(onDisk) != 3 {
		t.Fatalf("a save rewrote the file with %d records, want the live card and both archived ones: %v", len(onDisk), onDisk)
	}
	archivedByID := map[string]float64{}
	for _, rec := range onDisk {
		if rec["archived"] == true {
			id, ok := rec["id"].(string)
			if !ok {
				t.Fatalf("archived record without an id: %v", rec)
			}
			seconds, ok := rec["playtimeSeconds"].(float64)
			if !ok {
				t.Fatalf("archived record %q lost its playtime: %v", id, rec)
			}
			archivedByID[id] = seconds
		}
	}
	if !reflect.DeepEqual(archivedByID, map[string]float64{"a": 30, "b": 70}) {
		t.Fatalf("archived records after an unrelated save = %v, want a=30 b=70 untouched", archivedByID)
	}
}

func TestRemoveGameArchivesPlaytimeAndStatus(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	s := mustServiceAt(t, path)
	exe := tempGameExe(t)
	game, err := s.RegisterInstalled(InstalledGame{Title: "Gone", Executable: exe, InstallDir: filepath.Dir(exe), CanonicalGameID: "canon-gone"})
	if err != nil {
		t.Fatal(err)
	}
	played := time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC)
	s.mu.Lock()
	stored := s.findLocked(game.ID)
	stored.PlaytimeSeconds = 4200
	stored.LastPlayed = &played
	s.mu.Unlock()
	if _, err := s.SetStatus(game.ID, StatusCompleted); err != nil {
		t.Fatal(err)
	}

	if err := s.RemoveGame(game.ID); err != nil {
		t.Fatal(err)
	}

	if len(s.GetGames()) != 0 {
		t.Fatalf("removed game is still listed: %+v", s.GetGames())
	}
	for _, svc := range []*Service{s, mustServiceAt(t, path)} {
		var found *Game
		for _, g := range svc.GetHistoryGames() {
			if g.ID == game.ID {
				found = &g
			}
		}
		if found == nil {
			t.Fatal("removed game vanished from the play history")
		}
		if found.PlaytimeSeconds != 4200 || found.Status != StatusCompleted || found.CanonicalGameID != "canon-gone" ||
			found.LastPlayed == nil || !found.LastPlayed.Equal(played) || !found.Archived {
			t.Fatalf("archived record lost data: %+v", *found)
		}
		if found.InstallDir != "" || found.Executable != "" {
			t.Fatalf("archived record kept the installation: %+v", *found)
		}
	}
}

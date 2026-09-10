package install

import (
	"os"
	"path/filepath"
	"testing"

	"typhon/internal/settings"
)

func TestInstallerOwnershipSurvivesRetryAndRemoval(t *testing.T) {
	s, downloads, registrar := newTestService(t)
	setCleanupPolicy(t, s, settings.CleanupKeep)
	root := t.TempDir()
	innoSource(t, root)
	downloads.add("d1", "Game", root)
	games := t.TempDir()
	dest := filepath.Join(games, "Game")
	s.roots = []string{games}
	// Simulate an interrupted installer whose output must remain on disk.
	runner := &fakeRunner{err: errInstallerNotConfirmedStopped, act: func(runSpec) {
		mkFile(t, filepath.Join(dest, "Game.exe"), 8192)
	}}
	s.runner = runner
	item, err := s.Start("d1", StartOptions{Destination: dest})
	if err != nil {
		t.Fatal(err)
	}
	s.waitStatus(t, item.ID, StatusFailed)
	s.waitJobDone(t, item.ID)
	if missing(t, dest) {
		t.Fatal("interrupted output removed")
	}
	saved, err := s.store.load()
	if err != nil {
		t.Fatal(err)
	}
	if len(saved) != 1 || saved[0].OwnedDestination != dest {
		t.Fatalf("ownership not persisted: %+v", saved)
	}
	// Reload the persisted record so the retry cannot rely on transient state.
	s.mu.Lock()
	s.items = []*Installation{&saved[0]}
	s.mu.Unlock()
	runner.mu.Lock()
	runner.err = nil
	runner.mu.Unlock()
	if err := s.Retry(item.ID); err != nil {
		t.Fatal(err)
	}
	done := s.waitStatus(t, item.ID, StatusCompleted)
	s.waitJobDone(t, item.ID)
	if !done.Owned {
		t.Fatal("completed retry lost ownership")
	}
	game, err := registrar.Find(done.GameID)
	if err != nil || !game.Owned {
		t.Fatalf("registered game: %+v, %v", game, err)
	}
	info, err := s.InspectRemoval(done.GameID)
	if err != nil || !info.Owned || info.Method != RemovalFiles {
		t.Fatalf("removal info: %+v, %v", info, err)
	}
	if err := s.RemoveGame(done.GameID, RemoveOptions{DeleteFiles: true, KeepInLibrary: true}); err != nil {
		t.Fatal(err)
	}
	if !missing(t, dest) {
		t.Fatal("game files remain")
	}
	game, err = registrar.Find(done.GameID)
	if err != nil || !game.Uninstalled {
		t.Fatalf("library record: %+v, %v", game, err)
	}
}

func TestInstallerOwnershipTargetBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name            string
		empty, redirect bool
		want            bool
	}{
		{name: "existing empty target", empty: true, want: true},
		{name: "existing foreign files"},
		{name: "installer changed destination", empty: true, redirect: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, _, _ := newTestService(t)
			dest := t.TempDir()
			if !tc.empty {
				mkFile(t, filepath.Join(dest, "foreign.dat"), 10)
			}
			s.mu.Lock()
			s.items = append(s.items, &Installation{ID: "i1", Destination: dest})
			s.mu.Unlock()
			if err := s.rememberInstallerDestination("i1", dest); err != nil {
				t.Fatal(err)
			}
			actual := dest
			if tc.redirect {
				actual = t.TempDir()
				mkFile(t, filepath.Join(actual, "foreign.dat"), 10)
			}
			before := fsSnapshot{dirs: map[string]dirState{actual: {}}}
			if err := s.setRemoval("i1", actual, before, nil, "Game"); err != nil {
				t.Fatal(err)
			}
			got, _ := s.snapshot("i1")
			if got.Owned != tc.want {
				t.Fatalf("Owned=%v, want %v", got.Owned, tc.want)
			}
		})
	}
}

func TestResumedInstallerUsesPersistedDestination(t *testing.T) {
	for _, matches := range []bool{true, false} {
		s, _, _ := newTestService(t)
		dest := t.TempDir()
		owned := dest
		if !matches {
			owned = t.TempDir()
		}
		s.mu.Lock()
		s.items = append(s.items, &Installation{ID: "i1", Destination: dest, OwnedDestination: owned})
		s.mu.Unlock()
		if err := s.markResumedOwnership("i1"); err != nil {
			t.Fatal(err)
		}
		got, _ := s.snapshot("i1")
		if got.Owned != matches || !got.UninstallUnknown {
			t.Fatalf("resumed: %+v", got)
		}
	}
}

func TestInstallerOwnershipRollsBackWhenSaveFails(t *testing.T) {
	s, _, _ := newTestService(t)
	dest := filepath.Join(t.TempDir(), "Game")
	s.mu.Lock()
	s.items = append(s.items, &Installation{ID: "i1", Destination: dest})
	s.mu.Unlock()
	blocker := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocker, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	old := s.store.dir
	s.store.dir = blocker
	err := s.rememberInstallerDestination("i1", dest)
	s.store.dir = old
	if err == nil {
		t.Fatal("save failure ignored")
	}
	got, _ := s.snapshot("i1")
	if got.OwnedDestination != "" {
		t.Fatal("failed save not rolled back")
	}
}

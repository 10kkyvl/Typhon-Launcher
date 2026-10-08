package library

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestRemoveGameRollbackRestoresTheExactExclusionState(t *testing.T) {
	tests := []struct {
		name           string
		excludedBefore bool
		dirGone        bool
	}{
		{"excluded before, folder present", true, false},
		{"excluded before, folder gone", true, true},
		{"not excluded before, folder present", false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "library.json")
			s := mustServiceAt(t, path)
			dir, exe := discoveredDir(t, "Manual")
			if tt.excludedBefore {
				first, _, err := s.ApplyDiscovered(Discovered{Title: "Manual", Executable: exe, InstallDir: dir})
				if err != nil {
					t.Fatalf("apply: %v", err)
				}
				if err := s.RemoveGame(first.ID); err != nil {
					t.Fatalf("first removal: %v", err)
				}
			}
			manual, err := s.AddGame(exe, "Manual")
			if err != nil {
				t.Fatalf("add: %v", err)
			}
			if tt.dirGone {
				if err := os.RemoveAll(dir); err != nil {
					t.Fatal(err)
				}
			}
			s.mu.Lock()
			before := append([]string(nil), s.excluded...)
			s.mu.Unlock()
			if tt.excludedBefore && len(before) != 1 {
				t.Fatalf("precondition: want one exclusion, got %v", before)
			}
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(path, 0o755); err != nil {
				t.Fatal(err)
			}

			if err := s.RemoveGame(manual.ID); err == nil {
				t.Fatal("the library could not be saved but the removal reported success")
			}

			found := false
			for _, game := range s.GetGames() {
				found = found || game.ID == manual.ID
			}
			if !found {
				t.Fatal("the game is gone from memory although it was never removed")
			}
			s.mu.Lock()
			memory := append([]string(nil), s.excluded...)
			s.mu.Unlock()
			if !reflect.DeepEqual(memory, before) {
				t.Fatalf("exclusions in memory = %v, want the previous %v", memory, before)
			}
			disk, err := loadExcluded(s.excludedPath)
			if err != nil {
				t.Fatalf("load exclusions: %v", err)
			}
			if len(disk) != len(before) || (len(before) > 0 && !reflect.DeepEqual(disk, before)) {
				t.Fatalf("exclusions on disk = %v, want the previous %v", disk, before)
			}
		})
	}
}

func TestRegisterInstalledKeepsTheExclusionWhenTheGameCannotBeSaved(t *testing.T) {
	tests := []struct {
		name         string
		alreadyAdded bool
	}{
		{"a game that is not in the library yet", false},
		{"a game that is already in the library", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "library.json")
			s := mustServiceAt(t, path)
			dir, exe := discoveredDir(t, "Again")
			first, _, err := s.ApplyDiscovered(Discovered{Title: "Again", Executable: exe, InstallDir: dir})
			if err != nil {
				t.Fatalf("apply: %v", err)
			}
			if err := s.RemoveGame(first.ID); err != nil {
				t.Fatalf("first removal: %v", err)
			}
			if tt.alreadyAdded {
				if _, err := s.AddGame(exe, "Again"); err != nil {
					t.Fatalf("add: %v", err)
				}
			}
			s.mu.Lock()
			before := append([]string(nil), s.excluded...)
			games := len(s.games)
			s.mu.Unlock()
			if len(before) != 1 {
				t.Fatalf("precondition: want one exclusion, got %v", before)
			}
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(path, 0o755); err != nil {
				t.Fatal(err)
			}

			_, err = s.RegisterInstalled(InstalledGame{Title: "Again", Executable: exe, InstallDir: dir, InstallType: "portable", Owned: true})
			if err == nil {
				t.Fatal("the library could not be saved but the registration reported success")
			}

			s.mu.Lock()
			memory := append([]string(nil), s.excluded...)
			gotGames := len(s.games)
			s.mu.Unlock()
			if gotGames != games {
				t.Fatalf("games in memory = %d, want %d", gotGames, games)
			}
			if !reflect.DeepEqual(memory, before) {
				t.Fatalf("exclusions in memory = %v, want the previous %v", memory, before)
			}
			disk, err := loadExcluded(s.excludedPath)
			if err != nil {
				t.Fatalf("load exclusions: %v", err)
			}
			if !reflect.DeepEqual(disk, before) {
				t.Fatalf("exclusions on disk = %v, want the previous %v", disk, before)
			}
		})
	}
}

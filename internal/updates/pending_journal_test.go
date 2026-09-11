package updates

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"

	"typhon/internal/library"
)

type failingRollbackLibrary struct {
	librarySource
	err error
}

func (f *failingRollbackLibrary) ApplyInstalledUpdate(u library.InstalledUpdate) (library.Game, error) {
	return library.Game{}, f.err
}

func TestFailedRollbackPreservesJournalUntilRecovery(t *testing.T) {
	for _, attempt := range []string{"start_update", "swap_directories"} {
		t.Run(attempt, func(t *testing.T) {
			root := t.TempDir()
			current := filepath.Join(root, "game")
			previous := current + previousSuffix
			replaced := current + replacedSuffix
			for dir, version := range map[string]string{current: "new", previous: "old"} {
				mkTree(t, dir, version)
				if err := os.WriteFile(filepath.Join(dir, "game.exe"), []byte(version), 0600); err != nil {
					t.Fatal(err)
				}
			}
			libraryPath := filepath.Join(root, "library.json")
			lib, err := library.NewServiceAt(libraryPath)
			if err != nil {
				t.Fatal(err)
			}
			game, err := lib.RegisterInstalled(library.InstalledGame{Title: "Game", InstallDir: current, Executable: filepath.Join(current, "game.exe"), Version: "2", ReleaseID: "r2", SourceID: "s", DistributionID: "d"})
			if err != nil {
				t.Fatal(err)
			}
			configDir := filepath.Join(root, "updates")
			s, err := newServiceAt(configDir, nil)
			if err != nil {
				t.Fatal(err)
			}
			failure := errors.New("library persistence failed")
			s.library = &failingRollbackLibrary{librarySource: lib, err: failure}
			s.ctx = context.Background()
			s.downloads = newFakeDownloads()
			s.rollbacks[game.ID] = &Rollback{GameID: game.ID, Path: previous, InstallDir: current, Executable: game.Executable, Version: "1", ReleaseID: "r1", SourceID: "s", DistributionID: "d"}
			s.updates[game.ID] = &Update{GameID: game.ID, Plan: &UpdatePlan{GameID: game.ID}}
			if err := s.persistRollbacksLocked(); err != nil {
				t.Fatal(err)
			}
			if err := s.Rollback(game.ID); !errors.Is(err, failure) {
				t.Fatalf("rollback error = %v", err)
			}
			before, err := s.store.loadJournals()
			if err != nil || len(before) != 1 || before[0].Kind != JournalRollback {
				t.Fatalf("journal = %+v, error = %v", before, err)
			}
			if readMarker(t, current) != "old" || readMarker(t, replaced) != "new" {
				t.Fatal("failure did not occur after rollback renames")
			}
			inMemory := *s.journals[game.ID]
			switch attempt {
			case "start_update":
				if err := s.StartUpdate(game.ID); !errors.Is(err, errBusy) {
					t.Fatalf("start update error = %v, want busy", err)
				}
			case "swap_directories":
				if err := s.swapDirectories(game.ID, current, filepath.Join(root, "missing-staging"), previous, "3"); !errors.Is(err, errBusy) {
					t.Fatalf("swap error = %v, want busy", err)
				}
			}
			after, err := s.store.loadJournals()
			if err != nil || !reflect.DeepEqual(before, after) || s.journals[game.ID] == nil || !reflect.DeepEqual(*s.journals[game.ID], inMemory) {
				t.Fatalf("journal changed: %+v, error = %v", after, err)
			}
			if readMarker(t, current) != "old" || readMarker(t, replaced) != "new" {
				t.Fatal("retry destroyed a version")
			}
			onDisk, err := library.NewServiceAt(libraryPath)
			if err != nil {
				t.Fatal(err)
			}
			unchanged, err := onDisk.Find(game.ID)
			if err != nil || unchanged.Version != "2" {
				t.Fatalf("library before recovery = %+v, error = %v", unchanged, err)
			}
			fresh, err := newServiceAt(configDir, nil)
			if err != nil {
				t.Fatal(err)
			}
			fresh.library = onDisk
			if err := fresh.ServiceStartup(context.Background(), application.ServiceOptions{}); err != nil {
				t.Fatal(err)
			}
			if _, started := fresh.beginJob(game.ID); !started {
				t.Error("recovered game still blocked")
			} else {
				fresh.endJob(game.ID)
			}
			if err := fresh.ServiceShutdown(); err != nil {
				t.Fatal(err)
			}
			recovered, err := library.NewServiceAt(libraryPath)
			if err != nil {
				t.Fatal(err)
			}
			restored, err := recovered.Find(game.ID)
			if err != nil || restored.Version != "1" || restored.ReleaseID != "r1" || restored.Executable != game.Executable {
				t.Fatalf("recovered library = %+v, error = %v", restored, err)
			}
			if readMarker(t, current) != "old" || exists(replaced) || exists(previous) || fresh.HasRollback(game.ID) {
				t.Fatal("recovery left inconsistent files or metadata")
			}
			journals, err := fresh.store.loadJournals()
			if err != nil || len(journals) != 0 {
				t.Fatalf("recovered journals = %+v, error = %v", journals, err)
			}
		})
	}
}

func TestPendingJournalBlocksOnlyItsGame(t *testing.T) {
	for _, kind := range []string{JournalSwap, JournalRollback, JournalPatch, JournalInplace} {
		t.Run(string(kind), func(t *testing.T) {
			s, err := newServiceAt(t.TempDir(), nil)
			if err != nil {
				t.Fatal(err)
			}
			s.ctx = context.Background()
			if err := s.setJournal(SwapJournal{GameID: "pending", Kind: kind}); err != nil {
				t.Fatal(err)
			}
			if _, started := s.beginJob("pending"); started {
				s.endJob("pending")
				t.Error("job started with pending journal")
			}
			if _, started := s.beginJob("other"); !started {
				t.Error("unrelated game blocked")
			} else {
				s.endJob("other")
			}
		})
	}
}

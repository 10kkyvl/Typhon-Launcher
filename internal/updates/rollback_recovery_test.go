package updates

import (
	"context"
	"github.com/wailsapp/wails/v3/pkg/application"
	"os"
	"path/filepath"
	"testing"
	"time"
	"typhon/internal/library"
)

func TestAuditFix006RecoveryAtEachRollbackBoundary(t *testing.T) {
	for _, boundary := range []int{0, 1, 2} {
		t.Run(string(rune('0'+boundary)), func(t *testing.T) {
			root := t.TempDir()
			current := filepath.Join(root, "game")
			previous := current + previousSuffix
			replaced := current + replacedSuffix
			for dir, content := range map[string]string{current: "new", previous: "old"} {
				if err := os.MkdirAll(dir, 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dir, "game.exe"), []byte(content), 0600); err != nil {
					t.Fatal(err)
				}
			}
			lib, err := library.NewServiceAt(filepath.Join(t.TempDir(), "library.json"))
			if err != nil {
				t.Fatal(err)
			}
			game, err := lib.RegisterInstalled(library.InstalledGame{Title: "Game", InstallDir: current, Executable: filepath.Join(current, "game.exe"), Version: "2", ReleaseID: "r2", SourceID: "s", DistributionID: "d"})
			if err != nil {
				t.Fatal(err)
			}
			dir := t.TempDir()
			s, err := newServiceAt(dir, nil)
			if err != nil {
				t.Fatal(err)
			}
			s.library = lib
			entry := &Rollback{GameID: game.ID, Path: previous, InstallDir: current, Executable: filepath.Join(current, "game.exe"), Version: "1", ReleaseID: "r1", SourceID: "s", DistributionID: "d"}
			s.rollbacks[game.ID] = entry
			if err := s.persistRollbacksLocked(); err != nil {
				t.Fatal(err)
			}
			if err := s.setJournal(SwapJournal{GameID: game.ID, Kind: JournalRollback, InstallDir: current, Staging: previous, Previous: replaced, Rollback: entry, StartedAt: time.Now()}); err != nil {
				t.Fatal(err)
			}
			if boundary >= 1 {
				if err := os.Rename(current, replaced); err != nil {
					t.Fatal(err)
				}
			}
			if boundary >= 2 {
				if err := os.Rename(previous, current); err != nil {
					t.Fatal(err)
				}
			}
			fresh, err := newServiceAt(dir, nil)
			if err != nil {
				t.Fatal(err)
			}
			fresh.library = lib
			if err := fresh.ServiceStartup(context.Background(), application.ServiceOptions{}); err != nil {
				t.Fatal(err)
			}
			if err := fresh.ServiceShutdown(); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(filepath.Join(current, "game.exe"))
			if err != nil || string(data) != "old" {
				t.Fatalf("files=%q error=%v", data, err)
			}
			after, err := lib.Find(game.ID)
			if err != nil || after.ReleaseID != "r1" || after.Version != "1" {
				t.Fatalf("library=%+v error=%v", after, err)
			}
			if exists(previous) || exists(replaced) || fresh.HasRollback(game.ID) {
				t.Fatal("rollback left stale files or metadata")
			}
		})
	}
}

func TestAuditFix011RecoveryRetainsOlderRollback(t *testing.T) {
	for _, boundary := range []int{0, 1, 2, 3} {
		t.Run(string(rune('0'+boundary)), func(t *testing.T) {
			root := t.TempDir()
			current := filepath.Join(root, "game")
			previous := current + previousSuffix
			staging := current + ".staging"
			retained := previous + ".retained"
			for dir, content := range map[string]string{current: "v2", previous: "v1", staging: "v3"} {
				mkTree(t, dir, content)
			}
			j := SwapJournal{InstallDir: current, Previous: previous, Staging: staging, RetainedPrevious: retained}
			if boundary >= 1 {
				if err := os.Rename(previous, retained); err != nil {
					t.Fatal(err)
				}
			}
			if boundary >= 2 {
				if err := os.Rename(current, previous); err != nil {
					t.Fatal(err)
				}
			}
			if boundary >= 3 {
				if err := os.Rename(staging, current); err != nil {
					t.Fatal(err)
				}
			}
			if err := restoreSwapFiles(j); err != nil {
				t.Fatal(err)
			}
			if readMarker(t, current) != "v2" || readMarker(t, previous) != "v1" {
				t.Fatal("lost current or older rollback")
			}
		})
	}
}

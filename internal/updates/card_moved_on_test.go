package updates

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"typhon/internal/library"
)

// A journal left by a killed update or rollback names the folder the card had
// then. When the card names another folder since (a scan or an install rebound
// it), the next start still puts the files of that folder back, leaves the
// card where it is, and clears the journal: the card is not moved back to the
// old folder, and the game is not held by a journal no start can finish.
func TestJournalRecoveryLeavesACardThatNamesAnotherFolder(t *testing.T) {
	for _, kind := range []string{"swap", "rollback"} {
		t.Run(kind, func(t *testing.T) {
			root := t.TempDir()
			current := filepath.Join(root, "Games", "Game")
			previous := current + previousSuffix
			replaced := current + replacedSuffix
			mkTree(t, current, "new")
			mkTree(t, previous, "old")
			lib, err := library.NewServiceAt(filepath.Join(t.TempDir(), "library.json"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := lib.ServiceShutdown(); err != nil {
					t.Errorf("library shutdown: %v", err)
				}
			})
			game, err := lib.RegisterInstalled(t.Context(), library.InstalledGame{Title: "Game", InstallDir: current, Executable: filepath.Join(current, "marker.txt"), Version: "1", ReleaseID: "r1", SourceID: "s", DistributionID: "d"})
			if err != nil {
				t.Fatal(err)
			}
			dir := t.TempDir()
			s, err := newServiceAt(dir, nil)
			if err != nil {
				t.Fatal(err)
			}
			s.library = lib
			journal := SwapJournal{GameID: game.ID, Kind: JournalSwap, InstallDir: current, Staging: current + ".staging", Previous: previous, Version: "2", StartedAt: time.Now()}
			if kind == "rollback" {
				entry := &Rollback{GameID: game.ID, Path: previous, InstallDir: current, Executable: filepath.Join(current, "marker.txt"), Version: "0", ReleaseID: "r0", SourceID: "s", DistributionID: "d"}
				s.rollbacks[game.ID] = entry
				if err := s.persistRollbacksLocked(); err != nil {
					t.Fatal(err)
				}
				journal = SwapJournal{GameID: game.ID, Kind: JournalRollback, InstallDir: current, Staging: previous, Previous: replaced, Rollback: entry, StartedAt: time.Now()}
			}
			if err := s.setJournal(journal); err != nil {
				t.Fatal(err)
			}
			other := filepath.Join(root, "Elsewhere", "Game")
			mkTree(t, other, "the folder the card names now")
			rebound, err := lib.Relocate(game.ID, other)
			if err != nil {
				t.Fatal(err)
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

			if got := readMarker(t, current); got != "old" {
				t.Fatalf("old folder holds %q, want the version before the operation", got)
			}
			if got := readMarker(t, other); got != "the folder the card names now" {
				t.Fatalf("the card's folder holds %q", got)
			}
			card, err := lib.Find(game.ID)
			if err != nil {
				t.Fatal(err)
			}
			if card.InstallDir != rebound.InstallDir || card.Version != rebound.Version || card.ReleaseID != rebound.ReleaseID {
				t.Fatalf("card = %s v%s %s, want it where it was rebound: %s v%s %s", card.InstallDir, card.Version, card.ReleaseID, rebound.InstallDir, rebound.Version, rebound.ReleaseID)
			}
			journals, err := fresh.store.loadJournals()
			if err != nil {
				t.Fatal(err)
			}
			if len(journals) != 0 || fresh.HasRollback(game.ID) {
				t.Fatalf("journals = %+v, rollback held = %v; want the operation finished", journals, fresh.HasRollback(game.ID))
			}
			if _, err := os.Stat(replaced); err == nil {
				t.Fatalf("%s left behind", replaced)
			}
		})
	}
}

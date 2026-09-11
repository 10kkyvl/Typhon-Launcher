package updates

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"typhon/internal/library"

	"github.com/wailsapp/wails/v3/pkg/application"
)

func TestRetainedCleanupFailureResumesWithoutRollingBackCommittedInstall(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires Unix directory permissions")
	}
	for _, retry := range []string{"start", "restart"} {
		t.Run(retry, func(t *testing.T) {
			root := t.TempDir()
			current, previous := filepath.Join(root, "game"), filepath.Join(root, "game.previous")
			retained := previous + ".retained"
			for path, version := range map[string]string{current: "v3", previous: "v2", retained: "v1"} {
				mkTree(t, path, version)
			}
			s, err := newServiceAt(filepath.Join(root, "config"), nil)
			if err != nil {
				t.Fatal(err)
			}
			s.library = &fakeLibrary{games: []library.Game{{ID: "g", InstallDir: current, Version: "3"}}}
			if err := s.setJournal(SwapJournal{GameID: "g", Kind: JournalSwap, InstallDir: current, Previous: previous, RetainedPrevious: retained}); err != nil {
				t.Fatal(err)
			}
			//nolint:gosec // G302: retained is a directory; owner execute permission is required for the cleanup fixture.
			if err := os.Chmod(retained, 0500); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				//nolint:gosec // G302: retained is a directory; owner execute permission is required for the cleanup fixture.
				if err := os.Chmod(retained, 0700); err != nil && !os.IsNotExist(err) {
					t.Error(err)
				}
			})
			if err := s.clearJournal("g"); err != nil {
				t.Fatalf("committed update reported cleanup failure: %v", err)
			}
			journals, err := s.store.loadJournals()
			if err != nil || len(journals) != 1 || journals[0].Kind != JournalCleanup {
				t.Fatalf("journals=%+v err=%v", journals, err)
			}
			if readMarker(t, current) != "v3" || readMarker(t, previous) != "v2" {
				t.Fatal("cleanup changed committed installation")
			}
			//nolint:gosec // G302: retained is a directory; owner execute permission is required for the cleanup fixture.
			if err := os.Chmod(retained, 0700); err != nil {
				t.Fatal(err)
			}
			if retry == "start" {
				s.ctx = context.Background()
				if _, started := s.beginJob("g"); !started {
					t.Fatal("released backup still requires restart")
				} else {
					s.endJob("g")
				}
				if exists(retained) || len(s.journals) != 0 {
					t.Fatal("beginJob did not finish pending cleanup")
				}
			}
			fresh, err := newServiceAt(filepath.Join(root, "config"), nil)
			if err != nil {
				t.Fatal(err)
			}
			fresh.library = s.library
			if err := fresh.ServiceStartup(context.Background(), application.ServiceOptions{}); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := fresh.ServiceShutdown(); err != nil {
					t.Error(err)
				}
			})
			if exists(retained) || len(fresh.journals) != 0 {
				t.Fatal("retained cleanup not resumed")
			}
			if readMarker(t, current) != "v3" || readMarker(t, previous) != "v2" {
				t.Fatal("cleanup recovery rolled back committed installation")
			}
			if fresh.library.GetInstalledGames()[0].Version != "3" {
				t.Fatal("cleanup recovery rewrote library")
			}
			if _, started := fresh.beginJob("g"); !started {
				t.Fatal("cleanup left game blocked")
			} else {
				fresh.endJob("g")
			}
			staging := filepath.Join(root, "staging")
			mkTree(t, staging, "v4")
			if err := fresh.swapDirectories("g", current, staging, previous, "4"); err != nil {
				t.Fatalf("next swap remains blocked: %v", err)
			}

		})
	}
}

func TestSwapReclaimsLegacyRetainedOrphan(t *testing.T) {
	root := t.TempDir()
	current, previous, staging := filepath.Join(root, "game"), filepath.Join(root, "game.previous"), filepath.Join(root, "staging")
	for path, version := range map[string]string{current: "v3", previous: "v2", previous + ".retained": "v1", staging: "v4"} {
		mkTree(t, path, version)
	}
	s, err := newServiceAt(filepath.Join(root, "config"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.swapDirectories("g", current, staging, previous, "4"); err != nil {
		t.Fatal(err)
	}
	if readMarker(t, current) != "v4" || readMarker(t, previous) != "v3" || readMarker(t, previous+".retained") != "v2" {
		t.Fatal("orphan replacement lost an active version")
	}
}

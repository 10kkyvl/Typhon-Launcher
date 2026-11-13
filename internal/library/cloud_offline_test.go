package library_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"

	"typhon/internal/account"
	"typhon/internal/cloudsaves"
	"typhon/internal/library"
	"typhon/internal/savebackup"
	"typhon/internal/settings"
	"typhon/internal/uierr"
)

// The launcher starts with a conflict recorded in an earlier session and the
// cloud out of reach; the player chooses to play on this PC's saves. With
// the real library launch, savebackup's real hook and the cloud saves hook:
// the game starts once on the local saves, the pre-launch backup is taken
// before it, the conflict stays in the state file and holds the next Play.
func TestPlayAllowedOnceStartsOnTheLocalSavesWhileTheCloudIsDown(t *testing.T) {
	ctx := t.Context()
	lib, err := library.NewServiceAt(filepath.Join(t.TempDir(), "library.json"))
	if err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(t.TempDir(), "game.exe")
	if err := os.WriteFile(exe, []byte("test"), 0o600); err != nil {
		t.Fatal(err)
	}
	game, err := lib.AddGame(ctx, exe, "Game")
	if err != nil {
		t.Fatal(err)
	}
	library.SetCanonicalID(lib, game.ID, "cloud-1")
	saves := filepath.Join(t.TempDir(), "Saves")
	writeSaves(t, saves, map[string]string{"slot1.sav": "one", "profile/slot2.sav": "two"})
	if game, err = lib.SetSavesDir(game.ID, saves); err != nil {
		t.Fatal(err)
	}

	cfg := settings.Defaults()
	cfg.CloudSaves = true
	config := func() settings.Settings { return cfg }
	backups, err := savebackup.NewServiceAt(t.TempDir(), config, lib)
	if err != nil {
		t.Fatal(err)
	}
	if err := backups.ServiceStartup(ctx, application.ServiceOptions{}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := backups.ServiceShutdown(); err != nil {
			t.Errorf("savebackup shutdown: %v", err)
		}
	})
	fake := &cloud{copies: map[string]cloudCopy{}}
	stateDir := t.TempDir()
	newCloudSaves := func() *cloudsaves.Service {
		t.Helper()
		owner := func(string) (string, error) { return "account-1", nil }
		stored := func(string) string { return "account-1" }
		svc, err := cloudsaves.NewService(stateDir, config, func() (string, error) { return "token", nil }, owner, stored, fake, lib, backups)
		if err != nil {
			t.Fatal(err)
		}
		return svc
	}

	// The earlier session: synced once, then changed here and on another
	// device, and the conflict recorded.
	earlier := newCloudSaves()
	if err := earlier.ServiceStartup(ctx, application.ServiceOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := earlier.SyncNow(game.ID); err != nil {
		t.Fatalf("first sync: %v", err)
	}
	waitUntil(t, "the startup pass and the first sync", func() bool {
		lists, _ := fake.calls()
		return lists >= 2
	})
	// The startup pass lists under the lock every sync takes: one more sync
	// waits for that pass to end, so it cannot meet the changes made next.
	if err := earlier.SyncNow(game.ID); err != nil {
		t.Fatalf("sync after the startup pass: %v", err)
	}
	played := map[string]string{"slot1.sav": "one, played further", "profile/slot2.sav": "two"}
	writeSaves(t, saves, played)
	fake.put(t, "cloud-1", "other-device", map[string]string{"slot1.sav": "from the other device", "profile/slot2.sav": "two"})
	if err := earlier.SyncNow(game.ID); err != nil {
		t.Fatalf("sync recording the conflict: %v", err)
	}
	conflicted := false
	for _, g := range earlier.Status().Games {
		conflicted = conflicted || (g.GameID == game.ID && g.State == cloudsaves.StateConflict)
	}
	if !conflicted {
		t.Fatalf("setup: no conflict recorded, status %+v", earlier.Status().Games)
	}
	if err := earlier.ServiceShutdown(); err != nil {
		t.Fatalf("shutdown of the earlier session: %v", err)
	}

	// This session: the cloud cannot be reached from the start.
	fake.mu.Lock()
	fake.listErr = &account.Error{Code: account.CodeNetwork}
	listsBefore := fake.lists
	fake.mu.Unlock()
	cloudSaves := newCloudSaves()
	if err := cloudSaves.ServiceStartup(ctx, application.ServiceOptions{}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := cloudSaves.ServiceShutdown(); err != nil {
			t.Errorf("cloud saves shutdown: %v", err)
		}
	})
	waitUntil(t, "the startup pass", func() bool {
		lists, _ := fake.calls()
		return lists > listsBefore
	})

	type atStart struct {
		saves          map[string]string
		launchSnapshot bool
	}
	var starts []atStart
	library.StubLaunch(lib, ctx, func() error {
		files, err := readSaves(saves)
		if err != nil {
			return err
		}
		snaps, err := backups.List(ctx, game.ID)
		if err != nil {
			return err
		}
		taken := false
		for _, s := range snaps {
			taken = taken || s.Kind == savebackup.KindLaunch
		}
		starts = append(starts, atStart{files, taken})
		return os.ErrProcessDone
	})
	lib.AddLaunchHook(cloudSaves)
	lib.AddLaunchHook(backups)

	if err := lib.PlayGame(game.ID); uierr.Code(err) != "cloud.conflict_unresolved" {
		t.Fatalf("Play before the choice = %v, want the conflict hold", err)
	}
	if err := cloudSaves.AllowLaunchOnce(game.ID); err != nil {
		t.Fatalf("AllowLaunchOnce: %v", err)
	}
	lists, downloads := fake.calls()

	if err := lib.PlayGame(game.ID); uierr.Code(err) != "library.launch_failed" {
		t.Fatalf("the allowed Play = %v, want the stubbed start failure", err)
	}

	if len(starts) != 1 {
		t.Fatalf("the process was started %d times, want once", len(starts))
	}
	for name, want := range played {
		if starts[0].saves[name] != want {
			t.Fatalf("saves when the process started = %v, want the ones on this PC %v", starts[0].saves, played)
		}
	}
	if !starts[0].launchSnapshot {
		t.Fatal("no pre-launch backup when the process started: the save backup hook must run before it")
	}
	if l, d := fake.calls(); l != lists || d != downloads {
		t.Fatalf("%d listings and %d downloads for the allowed launch, want none", l-lists, d-downloads)
	}
	recorded := false
	for _, g := range newCloudSaves().Status().Games {
		recorded = recorded || (g.GameID == game.ID && g.State == cloudsaves.StateConflict && g.Conflict != nil)
	}
	if !recorded {
		t.Fatal("the state file lost the conflict after the allowed launch")
	}
	if err := lib.PlayGame(game.ID); uierr.Code(err) != "cloud.conflict_unresolved" {
		t.Fatalf("the next Play = %v, want the conflict hold again", err)
	}
	if len(starts) != 1 {
		t.Fatalf("the process was started %d times, want once", len(starts))
	}
}

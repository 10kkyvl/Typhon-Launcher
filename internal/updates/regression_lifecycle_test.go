package updates

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"
	"time"

	"typhon/internal/library"

	"github.com/wailsapp/wails/v3/pkg/application"
)

func startedService(t *testing.T, dir string, lib *fakeLibrary) *Service {
	t.Helper()
	svc, err := newServiceAt(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	if lib != nil {
		svc.library = lib
	}
	if err := svc.ServiceStartup(context.Background(), application.ServiceOptions{}); err != nil {
		t.Fatalf("ServiceStartup: %v", err)
	}
	t.Cleanup(func() {
		if err := svc.ServiceShutdown(); err != nil {
			t.Errorf("ServiceShutdown: %v", err)
		}
	})
	return svc
}

func historyIDs(list []UpdateHistory) []string {
	ids := make([]string, 0, len(list))
	for _, e := range list {
		ids = append(ids, e.ID)
	}
	return ids
}

func TestHistoryIsNewestFirstFilteredCappedAndSurvivesRestart(t *testing.T) {
	dir := t.TempDir()
	svc := startedService(t, dir, nil)

	if got := svc.GetHistory(""); len(got) != 0 {
		t.Fatalf("fresh history = %+v, want empty", got)
	}
	svc.appendHistory(UpdateHistory{ID: "h1", GameID: "a", Status: HistoryRunning})
	svc.appendHistory(UpdateHistory{ID: "h2", GameID: "b", Status: HistoryRunning})
	svc.appendHistory(UpdateHistory{ID: "h3", GameID: "a", Status: HistoryRunning})

	if got := historyIDs(svc.GetHistory("")); !slices.Equal(got, []string{"h3", "h2", "h1"}) {
		t.Fatalf("all history = %v, want newest first", got)
	}
	if got := historyIDs(svc.GetHistory("a")); !slices.Equal(got, []string{"h3", "h1"}) {
		t.Fatalf("history of a = %v, want only a's entries, newest first", got)
	}
	if got := svc.GetHistory("nobody"); len(got) != 0 {
		t.Fatalf("history of an unknown game = %+v, want empty", got)
	}

	svc.finishHistory("h1", HistoryCompleted, "")
	svc.finishHistory("h2", HistoryFailed, "disk full")
	svc.finishHistory("missing", HistoryFailed, "ignored")
	byID := map[string]UpdateHistory{}
	for _, e := range svc.GetHistory("") {
		byID[e.ID] = e
	}
	if e := byID["h1"]; e.Status != HistoryCompleted || e.CompletedAt == nil || e.Error != "" {
		t.Fatalf("h1 = %+v, want completed with a completion time", e)
	}
	if e := byID["h2"]; e.Status != HistoryFailed || e.Error != "disk full" || e.CompletedAt == nil {
		t.Fatalf("h2 = %+v, want failed with its reason", e)
	}
	if e := byID["h3"]; e.Status != HistoryRunning || e.CompletedAt != nil {
		t.Fatalf("h3 = %+v, want it untouched", e)
	}

	restarted := startedService(t, dir, nil)
	if got := historyIDs(restarted.GetHistory("")); !slices.Equal(got, []string{"h3", "h2", "h1"}) {
		t.Fatalf("history after restart = %v", got)
	}
	if e := restarted.GetHistory("b")[0]; e.Status != HistoryFailed || e.Error != "disk full" {
		t.Fatalf("finished entry lost its outcome on restart: %+v", e)
	}

	capped := startedService(t, t.TempDir(), nil)
	for i := range maxHistory + 5 {
		capped.appendHistory(UpdateHistory{ID: fmt.Sprintf("e%03d", i), GameID: "a"})
	}
	got := capped.GetHistory("")
	if len(got) != maxHistory {
		t.Fatalf("history length = %d, want it capped at %d", len(got), maxHistory)
	}
	if got[0].ID != fmt.Sprintf("e%03d", maxHistory+4) || got[len(got)-1].ID != "e005" {
		t.Fatalf("cap kept the wrong end: newest %q oldest %q", got[0].ID, got[len(got)-1].ID)
	}
}

func TestSweepPreviousRemovesOnlyWhatHasExpired(t *testing.T) {
	past := time.Now().Add(-time.Hour)
	future := time.Now().Add(time.Hour)
	tests := []struct {
		name        string
		entry       Rollback
		arrange     func(t *testing.T, f *persistFixture)
		wantRemoved bool
	}{
		{name: "expired", entry: Rollback{KeepUntil: &past}, wantRemoved: true},
		{name: "not yet expired", entry: Rollback{KeepUntil: &future}},
		{name: "kept until the next launch", entry: Rollback{AwaitLaunch: true}},
		{name: "expired but a rollback is running", entry: Rollback{KeepUntil: &past}, arrange: func(_ *testing.T, f *persistFixture) {
			f.svc.mu.Lock()
			defer f.svc.mu.Unlock()
			f.svc.rollbackActive = map[string]bool{f.game.ID: true}
		}},
		{name: "expired but a recovery journal is pending", entry: Rollback{KeepUntil: &past}, arrange: func(_ *testing.T, f *persistFixture) {
			f.svc.mu.Lock()
			defer f.svc.mu.Unlock()
			f.svc.journals[f.game.ID] = &SwapJournal{GameID: f.game.ID, Kind: JournalSwap}
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newPersistFixture(t)
			f.track(t)
			dir := f.seedRollback(t, tc.entry)
			if tc.arrange != nil {
				tc.arrange(t, f)
			}

			f.svc.sweepPrevious()

			_, kept := f.rollbackEntry(f.game.ID)
			if kept == tc.wantRemoved {
				t.Fatalf("rollback entry kept = %v, want removed = %v", kept, tc.wantRemoved)
			}
			if _, err := os.Stat(dir); (err == nil) == tc.wantRemoved {
				t.Fatalf("previous version present = %v, want removed = %v", err == nil, tc.wantRemoved)
			}
			u, _ := f.svc.snapshot(f.game.ID)
			if u.CanRollback == tc.wantRemoved {
				t.Fatalf("CanRollback = %v after sweep, want %v", u.CanRollback, !tc.wantRemoved)
			}
			stored, err := f.svc.store.loadRollbacks()
			if err != nil {
				t.Fatal(err)
			}
			if (len(stored) == 0) != tc.wantRemoved {
				t.Fatalf("rollbacks on disk = %+v, want them to match memory", stored)
			}
			storedUpdates, err := f.svc.store.loadUpdates()
			if err != nil || len(storedUpdates) != 1 || storedUpdates[0].CanRollback == tc.wantRemoved {
				t.Fatalf("updates on disk = %+v (%v), want CanRollback persisted as %v", storedUpdates, err, !tc.wantRemoved)
			}
		})
	}
}

func TestHandleSessionEndedDropsThePreviousVersionOnlyAfterARealLaunch(t *testing.T) {
	future := time.Now().Add(time.Hour)
	tests := []struct {
		name        string
		seconds     int64
		entry       *Rollback
		arrange     func(f *persistFixture)
		wantRemoved bool
	}{
		{name: "launch confirmed", seconds: 600, entry: &Rollback{AwaitLaunch: true}, wantRemoved: true},
		{name: "exactly the minimum session", seconds: 60, entry: &Rollback{AwaitLaunch: true}, wantRemoved: true},
		{name: "game crashed on start", seconds: 59, entry: &Rollback{AwaitLaunch: true}},
		{name: "zero-length session", seconds: 0, entry: &Rollback{AwaitLaunch: true}},
		{name: "kept by time, not by launch", seconds: 600, entry: &Rollback{KeepUntil: &future}},
		{name: "no previous version recorded", seconds: 600},
		{name: "a rollback is running", seconds: 600, entry: &Rollback{AwaitLaunch: true}, arrange: func(f *persistFixture) {
			f.svc.mu.Lock()
			defer f.svc.mu.Unlock()
			f.svc.rollbackActive = map[string]bool{f.game.ID: true}
		}},
		{name: "a recovery journal is pending", seconds: 600, entry: &Rollback{AwaitLaunch: true}, arrange: func(f *persistFixture) {
			f.svc.mu.Lock()
			defer f.svc.mu.Unlock()
			f.svc.journals[f.game.ID] = &SwapJournal{GameID: f.game.ID, Kind: JournalSwap}
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newPersistFixture(t)
			f.track(t)
			dir := filepath.Join(f.root, "Games", "Game.previous")
			if tc.entry != nil {
				dir = f.seedRollback(t, *tc.entry)
			}
			if tc.arrange != nil {
				tc.arrange(f)
			}

			f.svc.HandleSessionEnded(f.game.ID, tc.seconds)

			_, kept := f.rollbackEntry(f.game.ID)
			if tc.entry != nil && kept == tc.wantRemoved {
				t.Fatalf("rollback entry kept = %v, want removed = %v", kept, tc.wantRemoved)
			}
			if tc.entry == nil && kept {
				t.Fatal("a rollback entry appeared from nowhere")
			}
			if tc.entry != nil {
				if _, err := os.Stat(dir); (err == nil) == tc.wantRemoved {
					t.Fatalf("previous version present = %v, want removed = %v", err == nil, tc.wantRemoved)
				}
				if u, _ := f.svc.snapshot(f.game.ID); u.CanRollback == tc.wantRemoved {
					t.Fatalf("CanRollback = %v, want %v", u.CanRollback, !tc.wantRemoved)
				}
			}
		})
	}
}

func TestPruneForgetsGamesThatAreNoLongerInstalled(t *testing.T) {
	f := newPersistFixture(t)
	f.track(t)
	f.svc.emitVerify(f.game.ID, eventVerifyCompleted, func(v *VerifyState) {
		*v = VerifyState{GameID: f.game.ID, Method: MethodManifest}
	})

	f.svc.prune(map[string]bool{f.game.ID: true})
	if _, ok := f.svc.snapshot(f.game.ID); !ok {
		t.Fatal("prune dropped a game that is still installed")
	}

	f.svc.prune(map[string]bool{"another-game": true})
	if _, ok := f.svc.snapshot(f.game.ID); ok {
		t.Fatal("prune kept an update for a game that is gone")
	}
	f.svc.mu.Lock()
	remaining := len(f.svc.verifications)
	f.svc.mu.Unlock()
	if remaining != 0 {
		t.Fatalf("prune kept %d verification result(s) of a game that is gone", remaining)
	}
	updates, err := f.svc.store.loadUpdates()
	if err != nil || len(updates) != 0 {
		t.Fatalf("updates on disk = %+v (%v), want none", updates, err)
	}
	verifications, err := f.svc.store.loadVerifications()
	if err != nil || len(verifications) != 0 {
		t.Fatalf("verifications on disk = %+v (%v), want none", verifications, err)
	}
}

func TestRollbackRefusalsLeaveTheInstallationAlone(t *testing.T) {
	tests := []struct {
		name    string
		arrange func(t *testing.T, h *harness)
		want    error
		busy    bool
	}{
		{"nothing to roll back to", func(_ *testing.T, h *harness) {
			h.service.mu.Lock()
			defer h.service.mu.Unlock()
			delete(h.service.rollbacks, "local-1")
		}, errNoRollback, false},
		{"a recovery journal is pending", func(_ *testing.T, h *harness) {
			h.service.mu.Lock()
			defer h.service.mu.Unlock()
			h.service.journals["local-1"] = &SwapJournal{GameID: "local-1", Kind: JournalSwap}
		}, errBusy, false},
		{"another operation holds the game", func(t *testing.T, h *harness) {
			if _, ok := h.service.beginJob("local-1"); !ok {
				t.Fatal("setup: job slot unavailable")
			}
		}, errBusy, true},
		{"the game is running", func(_ *testing.T, h *harness) { h.library.running = []string{"local-1"} }, errGameRunning, false},
		{"the entry has no install directory", func(_ *testing.T, h *harness) {
			h.service.mu.Lock()
			defer h.service.mu.Unlock()
			h.service.rollbacks["local-1"].InstallDir = ""
		}, errEmptyInstallDir, false},
		{"the game was moved after the update", func(_ *testing.T, h *harness) {
			h.library.mu.Lock()
			defer h.library.mu.Unlock()
			h.library.games[0].InstallDir = filepath.Join(filepath.Dir(h.installDir), "Moved")
		}, errSwapFailed, false},
		{"the previous version was deleted", func(t *testing.T, h *harness) {
			if err := os.RemoveAll(h.installDir + previousSuffix); err != nil {
				t.Fatal(err)
			}
		}, errNoRollback, false},
		{"a half-finished rollback left its directory", func(t *testing.T, h *harness) {
			writeFile(t, h.installDir+replacedSuffix, "game.exe", "leftover")
		}, errSwapFailed, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t)
			h.plan(t)
			if err := h.service.StartUpdate("local-1"); err != nil {
				t.Fatal(err)
			}
			h.waitState(t, StateIdle)
			tc.arrange(t, h)

			err := h.service.Rollback("local-1")
			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
			if !tc.busy && h.service.Busy("local-1") {
				t.Fatal("a refused rollback left the game marked busy")
			}
			if data, err := os.ReadFile(filepath.Join(h.installDir, "game.exe")); err != nil || string(data) != "new executable" {
				t.Fatalf("a refused rollback touched the installation: %q %v", data, err)
			}
			if got := h.library.applied[len(h.library.applied)-1]; got.Version != "1.1" {
				t.Fatalf("a refused rollback told the library about %+v", got)
			}
		})
	}
}

func TestServiceStartupNormalisesWhatTheLastRunLeftBehind(t *testing.T) {
	dir := t.TempDir()
	root := t.TempDir()
	gamesDir := filepath.Join(root, "Games")
	installOf := func(id string) string { return filepath.Join(gamesDir, id) }
	stagingOf := func(id string) string { return filepath.Join(gamesDir, stagingDirName, id) }

	offer := UpdateAvailability{Available: true, Kind: KindUpdate, GameID: "x", TargetReleaseID: "r2", TargetVersion: "1.1"}
	plan := &UpdatePlan{ID: "p", TargetReleaseID: "r2"}
	stored := []Update{
		{GameID: "updating", State: StateUpdating, Step: StepSwap, Progress: 0.7, Availability: offer, Plan: plan},
		{GameID: "downloading", State: StateDownloading, Step: StepDownload, Progress: 0.3, Availability: offer, Plan: plan, Planning: true},
		{GameID: "available", State: StateAvailable, Availability: offer, Plan: plan},
		{GameID: "ready", State: StateReady, Availability: offer, Plan: plan},
		{GameID: "failed", State: StateFailed, Error: "диск заполнен", Availability: offer, Plan: plan},
		{GameID: "idle", State: StateIdle, Availability: offer},
		{GameID: "rollback", State: StateRollback, CanRollback: true, Availability: offer},
	}
	lib := &fakeLibrary{}
	for _, u := range stored {
		lib.games = append(lib.games, library.Game{ID: u.GameID, Title: u.GameID, InstallDir: installOf(u.GameID)})
		writeFile(t, stagingOf(u.GameID), "partial.bin", "half")
	}
	seed, err := newServiceAt(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := seed.store.saveUpdates(stored); err != nil {
		t.Fatal(err)
	}
	if err := seed.store.saveVerifications([]VerifyState{
		{GameID: "idle", Method: MethodTorrent, Running: true, Repairing: true, Progress: 0.5},
	}); err != nil {
		t.Fatal(err)
	}

	svc := startedService(t, dir, lib)

	want := map[string]struct {
		state State
		err   string
	}{
		"updating":    {StateFailed, interruptedUpdateText},
		"downloading": {StateFailed, interruptedUpdateText},
		"available":   {StateIdle, ""},
		"ready":       {StateIdle, ""},
		"failed":      {StateFailed, "диск заполнен"},
		"idle":        {StateIdle, ""},
		"rollback":    {StateRollback, ""},
	}
	for id, w := range want {
		u, err := svc.GetUpdate(id)
		if err != nil {
			t.Fatalf("%s: %v", id, err)
		}
		if u.State != w.state || u.Error != w.err {
			t.Errorf("%s: state = %q error = %q, want %q %q", id, u.State, u.Error, w.state, w.err)
		}
		if u.Plan != nil || u.Planning || u.Step != "" || u.Progress != 0 {
			t.Errorf("%s: plan/step survived the restart: %+v", id, u)
		}
		if !reflect.DeepEqual(u.Availability, UpdateAvailability{Kind: KindNone, GameID: id}) {
			t.Errorf("%s: a persisted offer was shown before it was re-checked: %+v", id, u.Availability)
		}
	}
	if u, err := svc.GetUpdate("rollback"); err != nil || !u.CanRollback {
		t.Errorf("restart dropped the rollback flag: %+v (%v)", u, err)
	}
	for _, id := range []string{"updating", "downloading", "failed"} {
		if _, err := os.Stat(stagingOf(id)); err == nil {
			t.Errorf("%s: staging left by the interrupted update was not cleaned up", id)
		}
	}
	for _, id := range []string{"available", "ready", "idle", "rollback"} {
		if _, err := os.Stat(stagingOf(id)); err != nil {
			t.Errorf("%s: staging of a game that was not interrupted was removed: %v", id, err)
		}
	}
	state, err := svc.GetVerifyState("idle")
	if err != nil {
		t.Fatal(err)
	}
	if state.Running || state.Repairing {
		t.Errorf("a verification that was running when the launcher died is still shown as running: %+v", state)
	}
}

func TestServiceStartupRefusesUnreadableStateAndLeavesItAlone(t *testing.T) {
	files := []string{"updates.json", "update_history.json", "rollbacks.json", "verify.json", "journal.json"}
	breakers := map[string]func(t *testing.T, path string){
		"corrupt": func(t *testing.T, path string) {
			if err := os.WriteFile(path, []byte(`{"version":1,"data":[{"gameId":`), 0o600); err != nil {
				t.Fatal(err)
			}
		},
		"empty": func(t *testing.T, path string) {
			if err := os.WriteFile(path, nil, 0o600); err != nil {
				t.Fatal(err)
			}
		},
		"unreadable": func(t *testing.T, path string) {
			if err := os.RemoveAll(path); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(path, 0o755); err != nil {
				t.Fatal(err)
			}
		},
		"from a newer launcher": func(t *testing.T, path string) {
			if err := os.WriteFile(path, []byte(`{"version":99,"data":[]}`), 0o600); err != nil {
				t.Fatal(err)
			}
		},
	}
	for _, file := range files {
		for kind, breakFile := range breakers {
			t.Run(file+"/"+kind, func(t *testing.T) {
				dir := t.TempDir()
				seed, err := newServiceAt(dir, nil)
				if err != nil {
					t.Fatal(err)
				}
				if err := seed.store.saveUpdates([]Update{{GameID: "g1", Title: "G"}}); err != nil {
					t.Fatal(err)
				}
				breakFile(t, filepath.Join(dir, file))
				before := snapshotUpdatesDir(t, dir)

				svc, err := newServiceAt(dir, nil)
				if err != nil {
					t.Fatal(err)
				}
				if err := svc.ServiceStartup(context.Background(), application.ServiceOptions{}); err == nil {
					t.Cleanup(func() {
						if err := svc.ServiceShutdown(); err != nil {
							t.Error(err)
						}
					})
					t.Fatal("ServiceStartup succeeded on unreadable state and will overwrite it on the next save")
				}
				if after := snapshotUpdatesDir(t, dir); !reflect.DeepEqual(before, after) {
					t.Fatalf("a refused start changed the state directory\nbefore %v\nafter  %v", before, after)
				}
				if got := svc.GetUpdates(); len(got) != 0 {
					t.Fatalf("a refused start exposed state: %+v", got)
				}
				svc.CheckUpdates()
				svc.HandleSessionEnded("g1", 600)
				svc.HandleSourcesRefreshed()
				if after := snapshotUpdatesDir(t, dir); !reflect.DeepEqual(before, after) {
					t.Fatalf("calls after a refused start changed the state directory\nbefore %v\nafter  %v", before, after)
				}
			})
		}
	}
}

func snapshotUpdatesDir(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	fsys := os.DirFS(dir)
	err := fs.WalkDir(fsys, ".", func(rel string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			out[rel] = "<dir>"
			return nil
		}
		data, readErr := fs.ReadFile(fsys, rel)
		if readErr != nil {
			return readErr
		}
		out[rel] = string(data)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

package updates

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"typhon/internal/library"
	"typhon/internal/sources"
)

type persistFixture struct {
	svc  *Service
	game library.Game
	root string
}

func newPersistFixture(t *testing.T) *persistFixture {
	t.Helper()
	root := t.TempDir()
	installDir := filepath.Join(root, "Games", "Game")
	writeFile(t, installDir, "game.exe", "old executable")
	game := library.Game{
		ID: "local-1", Title: "Game", CanonicalGameID: canonical,
		InstallDir: installDir, Executable: filepath.Join(installDir, "game.exe"),
		ReleaseID: "r1", SourceID: "src", DistributionID: "main",
		Version: "1.0", VersionSource: string(VersionSourceRelease),
	}
	svc, err := newServiceAt(filepath.Join(root, "config"), nil)
	if err != nil {
		t.Fatal(err)
	}
	svc.library = &fakeLibrary{games: []library.Game{game}}
	svc.releases = &fakeReleases{list: []sources.Release{release("r1", "1.0", 10<<20), release("r2", "1.1", 12<<20)}}
	svc.ctx, svc.cancel = context.WithCancel(context.Background())
	t.Cleanup(func() {
		svc.cancel()
		svc.wg.Wait()
	})
	return &persistFixture{svc: svc, game: game, root: root}
}

func (f *persistFixture) track(t *testing.T) {
	t.Helper()
	if err := f.svc.check(f.game); err != nil {
		t.Fatalf("seed check: %v", err)
	}
}

func (f *persistFixture) seedRollback(t *testing.T, entry Rollback) string {
	t.Helper()
	dir := filepath.Join(f.root, "Games", "Game.previous")
	writeFile(t, dir, "game.exe", "previous executable")
	entry.GameID = f.game.ID
	entry.Path = dir
	f.svc.mu.Lock()
	defer f.svc.mu.Unlock()
	f.svc.rollbacks[f.game.ID] = &entry
	if u, ok := f.svc.updates[f.game.ID]; ok {
		u.CanRollback = true
	}
	if err := f.svc.persistRollbacksLocked(); err != nil {
		t.Fatalf("seed rollbacks: %v", err)
	}
	if err := f.svc.persistLocked(); err != nil {
		t.Fatalf("seed updates: %v", err)
	}
	return dir
}

func (f *persistFixture) refuseWrites(t *testing.T, name string) {
	t.Helper()
	path := filepath.Join(f.svc.store.dir, name)
	if err := os.RemoveAll(path); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
}

func (f *persistFixture) allowWrites(t *testing.T, name string) {
	t.Helper()
	if err := os.Remove(filepath.Join(f.svc.store.dir, name)); err != nil {
		t.Fatal(err)
	}
}

func (f *persistFixture) degraded() bool {
	f.svc.mu.Lock()
	defer f.svc.mu.Unlock()
	return f.svc.status.Degraded
}

func (f *persistFixture) rollbackEntry(id string) (Rollback, bool) {
	f.svc.mu.Lock()
	defer f.svc.mu.Unlock()
	entry, ok := f.svc.rollbacks[id]
	if !ok {
		return Rollback{}, false
	}
	return *entry, true
}

func TestStateChangesRollBackWhenTheStateFileRefusesTheWrite(t *testing.T) {
	past := time.Now().Add(-time.Hour)
	tests := []struct {
		name  string
		file  string
		track bool
		seed  func(t *testing.T, f *persistFixture)
		act   func(t *testing.T, f *persistFixture)
		check func(t *testing.T, f *persistFixture, before Update)
	}{
		{
			name: "mutate", file: "updates.json", track: true,
			act: func(t *testing.T, f *persistFixture) {
				before, _ := f.svc.snapshot(f.game.ID)
				got, ok := f.svc.mutate(f.game.ID, func(u *Update) { u.Message = "changed" })
				if !ok || got != before {
					t.Fatalf("mutate returned %+v (ok=%v), want the unchanged %+v", got, ok, before)
				}
			},
			check: func(t *testing.T, f *persistFixture, before Update) {
				if after, _ := f.svc.snapshot(f.game.ID); after != before {
					t.Fatalf("memory = %+v, want rollback to %+v", after, before)
				}
			},
		},
		{
			name: "first check of a game", file: "updates.json",
			act: func(t *testing.T, f *persistFixture) {
				if err := f.svc.check(f.game); err == nil {
					t.Fatal("check reported success although nothing was saved")
				}
			},
			check: func(t *testing.T, f *persistFixture, _ Update) {
				if _, ok := f.svc.snapshot(f.game.ID); ok {
					t.Fatal("a failed first check left a half-tracked entry behind")
				}
			},
		},
		{
			name: "repeated check", file: "updates.json", track: true,
			act: func(t *testing.T, f *persistFixture) {
				f.svc.releases = &fakeReleases{list: []sources.Release{release("r1", "1.0", 10<<20), release("r2", "1.1", 12<<20), release("r3", "1.2", 13<<20)}}
				if _, err := f.svc.CheckGame(f.game.ID); err == nil {
					t.Fatal("CheckGame hid the failed save from the caller")
				}
			},
			check: func(t *testing.T, f *persistFixture, before Update) {
				if after, _ := f.svc.snapshot(f.game.ID); after != before {
					t.Fatalf("memory = %+v, want rollback to %+v", after, before)
				}
			},
		},
		{
			name: "prune", file: "updates.json", track: true,
			act: func(t *testing.T, f *persistFixture) { f.svc.prune(map[string]bool{}) },
			check: func(t *testing.T, f *persistFixture, before Update) {
				if after, ok := f.svc.snapshot(f.game.ID); !ok || after != before {
					t.Fatalf("prune removed an entry that is still on disk: %+v (ok=%v)", after, ok)
				}
			},
		},
		{
			name: "session end drops the kept previous version", file: "rollbacks.json", track: true,
			seed: func(t *testing.T, f *persistFixture) { f.seedRollback(t, Rollback{AwaitLaunch: true}) },
			act:  func(t *testing.T, f *persistFixture) { f.svc.HandleSessionEnded(f.game.ID, 600) },
			check: func(t *testing.T, f *persistFixture, _ Update) {
				if _, ok := f.rollbackEntry(f.game.ID); !ok {
					t.Fatal("rollback entry dropped in memory although the removal was not saved")
				}
				if u, _ := f.svc.snapshot(f.game.ID); !u.CanRollback {
					t.Fatal("CanRollback cleared although the removal was not saved")
				}
				if _, err := os.Stat(filepath.Join(f.root, "Games", "Game.previous")); err != nil {
					t.Fatalf("previous version deleted although the removal was not saved: %v", err)
				}
			},
		},
		{
			name: "sweep of an expired previous version", file: "rollbacks.json", track: true,
			seed: func(t *testing.T, f *persistFixture) { f.seedRollback(t, Rollback{KeepUntil: &past}) },
			act:  func(t *testing.T, f *persistFixture) { f.svc.sweepPrevious() },
			check: func(t *testing.T, f *persistFixture, _ Update) {
				if _, ok := f.rollbackEntry(f.game.ID); !ok {
					t.Fatal("rollback entry dropped in memory although the removal was not saved")
				}
				if _, err := os.Stat(filepath.Join(f.root, "Games", "Game.previous")); err != nil {
					t.Fatalf("previous version deleted although the removal was not saved: %v", err)
				}
			},
		},
		{
			name: "appending history", file: "update_history.json",
			act: func(t *testing.T, f *persistFixture) {
				f.svc.appendHistory(UpdateHistory{ID: "h1", GameID: f.game.ID, Status: HistoryRunning})
			},
			check: func(t *testing.T, f *persistFixture, _ Update) {
				if got := f.svc.GetHistory(""); len(got) != 0 {
					t.Fatalf("history = %+v, want the unsaved entry rolled back", got)
				}
			},
		},
		{
			name: "finishing history", file: "update_history.json",
			seed: func(t *testing.T, f *persistFixture) {
				f.svc.appendHistory(UpdateHistory{ID: "h1", GameID: f.game.ID, Status: HistoryRunning})
			},
			act: func(t *testing.T, f *persistFixture) { f.svc.finishHistory("h1", HistoryCompleted, "") },
			check: func(t *testing.T, f *persistFixture, _ Update) {
				got := f.svc.GetHistory("")
				if len(got) != 1 || got[0].Status != HistoryRunning || got[0].CompletedAt != nil {
					t.Fatalf("history = %+v, want the running entry untouched", got)
				}
			},
		},
		{
			name: "verification progress", file: "verify.json",
			seed: func(t *testing.T, f *persistFixture) {
				f.svc.emitVerify(f.game.ID, eventVerifyStarted, func(v *VerifyState) {
					*v = VerifyState{GameID: f.game.ID, Method: MethodManifest, Running: true}
				})
			},
			act: func(t *testing.T, f *persistFixture) {
				snap := f.svc.emitVerify(f.game.ID, eventVerifyCompleted, func(v *VerifyState) {
					v.Running = false
					v.Error = "boom"
				})
				if snap.Error != "" || !snap.Running {
					t.Fatalf("emitVerify returned %+v, want the unchanged running state", snap)
				}
			},
			check: func(t *testing.T, f *persistFixture, _ Update) {
				f.svc.mu.Lock()
				defer f.svc.mu.Unlock()
				if v := f.svc.verifications[f.game.ID]; v == nil || v.Error != "" || !v.Running {
					t.Fatalf("verification = %+v, want rollback to the running state", v)
				}
			},
		},
		{
			name: "remembering a previous version", file: "rollbacks.json", track: true,
			act: func(t *testing.T, f *persistFixture) {
				f.svc.rememberPrevious(f.game, filepath.Join(f.root, "Games", "Game.previous"))
			},
			check: func(t *testing.T, f *persistFixture, _ Update) {
				if _, ok := f.rollbackEntry(f.game.ID); ok {
					t.Fatal("rollback entry recorded in memory although it was not saved")
				}
				if u, _ := f.svc.snapshot(f.game.ID); u.CanRollback {
					t.Fatal("CanRollback set although the rollback was not saved")
				}
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newPersistFixture(t)
			if tc.track {
				f.track(t)
			}
			if tc.seed != nil {
				tc.seed(t, f)
			}
			before, _ := f.svc.snapshot(f.game.ID)
			f.refuseWrites(t, tc.file)

			tc.act(t, f)
			tc.check(t, f, before)
			if !f.degraded() {
				t.Fatal("the service did not report that its state could not be saved")
			}

			f.allowWrites(t, tc.file)
			if err := f.svc.check(f.game); err != nil {
				t.Fatalf("check after the disk recovered: %v", err)
			}
			if f.degraded() {
				t.Fatal("the degraded flag stayed on after a save succeeded")
			}
		})
	}
}

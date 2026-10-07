package install

import (
	"errors"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"typhon/internal/library"
)

// lockedLibrary — библиотека, которая пока заперта и отказывается регистрировать игру.
type lockedLibrary struct {
	*fakeRegistrar
	locked atomic.Bool
}

func (l *lockedLibrary) RegisterInstalled(g library.InstalledGame) (library.Game, error) {
	if l.locked.Load() {
		return library.Game{}, errors.New("library is locked")
	}
	return l.fakeRegistrar.RegisterInstalled(g)
}

type retryPlan struct {
	opts       StartOptions
	failCode   string
	failText   string
	afterFail  func(t *testing.T, r *rig)
	fix        func()
	wantStatus Status
	confirm    string
	exe        string
	chain      int
	attempts   int
	run        *rgRunner
}

type retryCase struct {
	name  string
	build func(t *testing.T, r *rig) retryPlan
}

func retryCases() []retryCase {
	return []retryCase{
		{"silent installer exits with an error", func(t *testing.T, r *rig) retryPlan {
			dest := silentSource(t, r, rgInnoMarker)
			r.setRunner(newRgRunner(t, rgStep{code: 1}))
			return retryPlan{
				opts: StartOptions{Destination: dest}, failCode: "install.installer_failed", wantStatus: StatusCompleted,
				exe: filepath.Join(dest, "Game.exe"),
				fix: func() { r.setRunner(newRgRunner(t, rgStep{act: installGame(t, dest)})) },
			}
		}},
		{"second installer of a chain fails", func(t *testing.T, r *rig) retryPlan {
			root := t.TempDir()
			cupheadSource(t, root)
			r.download("d1", "Cuphead", root)
			dest := filepath.Join(r.games, "Cuphead")
			install := func(runSpec) { mkFile(t, filepath.Join(dest, "Cuphead.exe"), 8192) }
			r.setRunner(newRgRunner(t, rgStep{act: install}, rgStep{code: 1}))
			second := newRgRunner(t, rgStep{act: install})
			return retryPlan{
				opts: StartOptions{Destination: dest}, failCode: "install.installer_failed", wantStatus: StatusCompleted,
				exe: filepath.Join(dest, "Cuphead.exe"), chain: 2, attempts: 2, run: second,
				fix: func() { r.setRunner(second) },
			}
		}},
		{"zip fails its checksum and is downloaded again", func(t *testing.T, r *rig) retryPlan {
			var zipPath string
			dest := archiveSource(t, r, "game.zip", func(path string) {
				zipPath = path
				storedZip(t, path, []zipEntry{{name: "Game/Game.exe", data: []byte(rgCorruptMarker + strings.Repeat("g", 4096))}})
				corruptStoredZip(t, path)
			})
			return retryPlan{
				opts: StartOptions{Destination: dest}, failText: "checksum", wantStatus: StatusCompleted,
				exe: filepath.Join(dest, "Game.exe"),
				fix: func() { writeZip(t, zipPath, gameZipEntries()) },
			}
		}},
		{"portable files are in place but the library refused the game", func(t *testing.T, r *rig) retryPlan {
			root := t.TempDir()
			portableSource(t, root, "Game")
			r.download("d1", "Game", root)
			dest := filepath.Join(t.TempDir(), "Games", "Game")
			lib := &lockedLibrary{fakeRegistrar: r.reg}
			lib.locked.Store(true)
			r.lib = lib
			r.s.library = lib
			return retryPlan{
				opts: StartOptions{Destination: dest, Mode: ModeCopy}, failText: "library is locked", wantStatus: StatusCompleted,
				exe: filepath.Join(dest, "Game.exe"),
				afterFail: func(t *testing.T, _ *rig) {
					if !exists(filepath.Join(dest, "Game.exe")) {
						t.Fatal("the committed files were removed because registration failed")
					}
				},
				fix: func() { lib.locked.Store(false) },
			}
		}},
		{"interactive installer exits with an error", func(t *testing.T, r *rig) retryPlan {
			installed := interactiveSource(t, r)
			r.setRunner(newRgRunner(t, rgStep{code: 1}))
			exe := filepath.Join(installed, "MyGame.exe")
			return retryPlan{
				failCode: "install.installer_failed", wantStatus: StatusWaitingForUser, confirm: exe, exe: exe,
				fix: func() {
					r.setRunner(newRgRunner(t, rgStep{act: func(runSpec) { mkFile(t, exe, 512<<10) }}))
				},
			}
		}},
		{"silent installer in the elevated worker exits with an error", func(t *testing.T, r *rig) retryPlan {
			dest := silentSource(t, r, rgInnoMarker)
			failing := &rgWorker{t: t, act: func(workerSpec) workerState { return workerState{Code: 1} }}
			failing.install()
			r.setRunner(elevatingRunner{})
			return retryPlan{
				opts: StartOptions{Destination: dest}, failCode: "install.installer_failed", wantStatus: StatusCompleted,
				exe: filepath.Join(dest, "Game.exe"),
				fix: func() {
					working := &rgWorker{t: t, act: func(workerSpec) workerState {
						mkFile(t, filepath.Join(dest, "Game.exe"), 8192)
						return workerState{}
					}}
					working.install()
				},
			}
		}},
	}
}

func TestFlowRetryAfterFailure(t *testing.T) {
	for _, tc := range retryCases() {
		t.Run(tc.name, func(t *testing.T) {
			r := newRig(t)
			p := tc.build(t, r)
			item, err := r.s.Start("d1", p.opts)
			if err != nil {
				t.Fatalf("Start: %v", err)
			}
			failed := r.settle(item.ID)
			if failed.Status != StatusFailed || uiCode(failed.Error) != p.failCode || !strings.Contains(failed.Error, p.failText) {
				t.Fatalf("first attempt: status %s error %q, want a failure %q %q", failed.Status, failed.Error, p.failCode, p.failText)
			}
			if p.afterFail != nil {
				p.afterFail(t, r)
			}
			if games := r.reg.registered(); len(games) != 0 {
				t.Fatalf("a failed install registered %+v", games)
			}

			p.fix()
			if err := r.s.Retry(item.ID); err != nil {
				t.Fatalf("Retry: %v", err)
			}
			got := r.settle(item.ID)
			if got.Status != p.wantStatus || got.Error != "" {
				t.Fatalf("after Retry: status %s error %q, want %s", got.Status, got.Error, p.wantStatus)
			}
			if got.ID != item.ID || len(r.s.List()) != 1 {
				t.Fatalf("Retry must reuse the record: %d records", len(r.s.List()))
			}
			if p.confirm != "" {
				if err := r.s.ConfirmExecutable(item.ID, p.confirm); err != nil {
					t.Fatalf("ConfirmExecutable: %v", err)
				}
				got = r.get(item.ID)
			}
			if got.Status != StatusCompleted || got.Executable != p.exe || got.ChainStep != p.chain || got.CompletedAt == nil || got.GameID == "" {
				t.Fatalf("after the retry: %+v", viewOf(got))
			}
			if games := r.reg.registered(); len(games) != 1 {
				t.Fatalf("registered %d games, want exactly 1", len(games))
			}
			if p.run != nil && len(p.run.calls()) != p.attempts {
				t.Fatalf("installers run by the retry = %d, want the whole chain (%d)", len(p.run.calls()), p.attempts)
			}
			if err := r.s.Retry(item.ID); errCode(err) != "install.unavailable" {
				t.Fatalf("Retry of a completed install = %v, want install.unavailable", err)
			}
			r.assertDurable(item.ID)
		})
	}
}

// Удаление игры забывает и записи установки: иначе история установок
// ссылается на игру, которой уже нет, и после перезапуска возвращается.
func TestRemoveGameForgetsItsInstallations(t *testing.T) {
	r := newRig(t)
	root := t.TempDir()
	portableSource(t, root, "Game")
	r.download("d1", "Game", root)
	dest := filepath.Join(t.TempDir(), "Games", "Game")
	item, err := r.s.Start("d1", StartOptions{Destination: dest, Mode: ModeCopy})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	done := r.settle(item.ID)
	if done.Status != StatusCompleted {
		t.Fatalf("status = %s (%q)", done.Status, done.Error)
	}

	if err := r.s.RemoveGame(done.GameID, RemoveOptions{DeleteFiles: true}); err != nil {
		t.Fatalf("RemoveGame: %v", err)
	}
	if exists(dest) {
		t.Fatalf("%s survived the removal of an owned install", dest)
	}
	if got := r.s.List(); len(got) != 0 {
		t.Fatalf("installation records after removal = %+v", got)
	}
	if got := r.disk(); len(got) != 0 {
		t.Fatalf("installations.json after removal = %+v", got)
	}
	if got := r.reg.removed; len(got) != 1 || got[0] != done.GameID {
		t.Fatalf("library removals = %v", got)
	}
	if left := r.stateFiles(); len(left) != 0 {
		t.Fatalf("state directory after removal holds %v", left)
	}
	r.restart()
	if got := r.s.List(); len(got) != 0 {
		t.Fatalf("a removed install came back after a restart: %+v", got)
	}
}

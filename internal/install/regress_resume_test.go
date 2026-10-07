package install

import (
	"path/filepath"
	"strings"
	"testing"
)

func (r *rig) addResumable(id string) Installation {
	r.t.Helper()
	dest := filepath.Join(r.games, "Game")
	exe := filepath.Join(dest, "Game.exe")
	mkFile(r.t, exe, 4096)
	item := Installation{
		ID: id, DownloadID: "d1", Name: "Game", Type: TypeExeInstaller, Status: StatusInstalling,
		Destination: dest, Executable: exe, Engine: EngineInno, Silent: true, OwnedDestination: dest,
	}
	r.add(item)
	return item
}

// Итог воркера, пережившего перезапуск, обязан дойти до записи, даже если диск
// отказывает: запись не остаётся «в работе» навсегда и не объявляется готовой.
func TestResumedInstallReportsPersistFailure(t *testing.T) {
	cases := []struct {
		name string
		do   func(r *rig, id string)
	}{
		{"worker finished, ownership cannot be saved", func(r *rig, id string) {
			r.s.finishResumed(r.s.ctx, id, workerState{Done: true})
		}},
		{"worker disappeared, interruption cannot be saved", func(r *rig, id string) {
			r.s.interruptResumed(id)
		}},
		{"worker confirmed the cancel, cancellation cannot be saved", func(r *rig, id string) {
			r.s.cancelResumed(r.s.ctx, id)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newRig(t)
			item := r.addResumable("res1")
			unblock := r.blockStore()

			tc.do(r, item.ID)

			got := r.get(item.ID)
			if got.Status != StatusFailed || !strings.Contains(got.Error, errPersistPrefix) {
				t.Fatalf("status %s error %q, want a failure that carries the save error to the UI", got.Status, got.Error)
			}
			if games := r.reg.registered(); len(games) != 0 {
				t.Fatalf("registered %+v although the install state could not be saved", games)
			}
			unblock()
		})
	}
}

func TestResumedInstallFinishedByTheWorkerIsRegisteredOnceWithUnknownOrigin(t *testing.T) {
	r := newRig(t)
	item := r.addResumable("res2")

	r.s.finishResumed(r.s.ctx, item.ID, workerState{Done: true})

	got := r.get(item.ID)
	if got.Status != StatusCompleted || got.GameID == "" || got.Error != "" {
		t.Fatalf("after the worker finished: %+v", viewOf(got))
	}
	if !got.UninstallUnknown || !got.Owned {
		t.Fatalf("owned %v uninstallUnknown %v: the folder is ours (recorded before the run), the uninstaller is not known", got.Owned, got.UninstallUnknown)
	}
	games := r.reg.registered()
	if len(games) != 1 || games[0].InstallDir != item.Destination || games[0].Executable != item.Executable {
		t.Fatalf("registered = %+v", games)
	}
	r.assertDurable(item.ID)
}

func TestResumedInstallThatWasNotCreatedByUsStaysUnowned(t *testing.T) {
	r := newRig(t)
	item := r.addResumable("res3")
	r.s.mu.Lock()
	r.s.findLocked(item.ID).OwnedDestination = ""
	r.s.mu.Unlock()

	r.s.finishResumed(r.s.ctx, item.ID, workerState{Done: true})

	got := r.get(item.ID)
	if got.Status != StatusCompleted || got.Owned {
		t.Fatalf("status %s owned %v: a folder nobody recorded as empty before the run must never be deleted wholesale", got.Status, got.Owned)
	}
}

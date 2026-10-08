//go:build windows

package install

import (
	"errors"
	"os"
	"sync/atomic"
	"testing"
	"time"
)

// Эти тесты берут настоящий процесс (этот же) за живой воркер: workerProcessAlive
// вне Windows всегда отвечает «нет», и сценарий «воркер жив» там не воспроизвести.

func fastResumePolling(t *testing.T) {
	t.Helper()
	previous := resumeWatchPollInterval
	resumeWatchPollInterval = 10 * time.Millisecond
	t.Cleanup(func() { resumeWatchPollInterval = previous })
}

func (r *rig) startWithLiveWorker(id string, state workerState) Installation {
	r.t.Helper()
	dest := r.games + string(os.PathSeparator) + "Game"
	item := Installation{
		ID: id, DownloadID: "d1", Name: "Game", Type: TypeExeInstaller, Status: StatusInstalling,
		Destination: dest, Engine: EngineInno, Silent: true, OwnedDestination: dest,
	}
	r.seedWith(func() {
		if err := writeWorkerState(workerStatePath(r.dir, id), state); err != nil {
			r.t.Fatalf("write worker state: %v", err)
		}
	}, item)
	if got := r.get(id); got.Status != StatusInstalling {
		r.t.Fatalf("status after startup = %s, want the install to wait for its live worker", got.Status)
	}
	return item
}

func TestResumedInstallIsInterruptedWhenTheWorkerGoesAway(t *testing.T) {
	cases := []struct {
		name   string
		strike func(t *testing.T, r *rig, id string)
	}{
		{"state file disappears", func(t *testing.T, r *rig, id string) {
			if err := removeWithRetry(workerStatePath(r.dir, id)); err != nil {
				t.Fatalf("remove worker state: %v", err)
			}
		}},
		{"worker process dies without a result", func(t *testing.T, r *rig, id string) {
			if err := writeWorkerState(workerStatePath(r.dir, id), workerState{PID: 999999999}); err != nil {
				t.Fatalf("write worker state: %v", err)
			}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fastResumePolling(t)
			r := newRig(t)
			r.startWithLiveWorker("live1", workerState{PID: os.Getpid()})

			tc.strike(t, r, "live1")

			got := r.s.waitStatus(t, "live1", StatusInterrupted)
			if got.Error != errInterrupted.Error() {
				t.Fatalf("error = %q", got.Error)
			}
			if disk := r.diskItem("live1"); disk.Status != StatusInterrupted {
				t.Fatalf("installations.json holds %s, want interrupted", disk.Status)
			}
		})
	}
}

func TestResumedInstallGivesUpOnAStateFileThatStaysUnreadable(t *testing.T) {
	fastResumePolling(t)
	var broken atomic.Bool
	previous := readWorkerStateForResume
	readWorkerStateForResume = func(path string) (workerState, bool, error) {
		if broken.Load() {
			return workerState{}, false, errors.New("state file is locked")
		}
		return previous(path)
	}
	t.Cleanup(func() { readWorkerStateForResume = previous })

	r := newRig(t)
	r.startWithLiveWorker("live2", workerState{PID: os.Getpid()})
	broken.Store(true)

	got := r.s.waitStatus(t, "live2", StatusInterrupted)
	if got.Error != errInterrupted.Error() {
		t.Fatalf("error = %q", got.Error)
	}
}

// Cancel над записью, подхваченной после перезапуска, не врёт «отменено», пока
// воркер жив: запись остаётся рабочей, а воркеру уходит маркер.
func TestCancelOfAResumedInstallLeavesItWorkingUntilTheWorkerConfirms(t *testing.T) {
	fastResumePolling(t)
	r := newRig(t)
	r.startWithLiveWorker("live3", workerState{PID: os.Getpid()})

	if err := r.s.Cancel("live3"); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if got := r.get("live3"); got.Status != StatusInstalling {
		t.Fatalf("status = %s right after Cancel, want the install still working", got.Status)
	}
	if !workerCancelRequested(workerCancelPath(r.dir, "live3")) {
		t.Fatal("the worker was not asked to stop")
	}

	done := workerState{PID: os.Getpid(), Done: true, Cancelled: true, Error: "context canceled"}
	if err := writeWorkerState(workerStatePath(r.dir, "live3"), done); err != nil {
		t.Fatalf("write worker state: %v", err)
	}
	if got := r.s.waitStatus(t, "live3", StatusCancelled); got.Error != "" {
		t.Fatalf("error = %q on a cancelled install", got.Error)
	}
}

package install

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func workerFilesOf(r *rig) []string {
	r.t.Helper()
	var out []string
	for _, name := range r.stateFiles() {
		if strings.HasPrefix(name, "worker-") {
			out = append(out, name)
		}
	}
	return out
}

func seedWorkerFiles(t *testing.T, dir, id string, state workerState) []string {
	t.Helper()
	if err := writeWorkerSpec(workerSpecFilePath(dir, id), workerSpec{ID: id, Run: "old"}); err != nil {
		t.Fatal(err)
	}
	if err := writeWorkerState(workerStatePath(dir, id), state); err != nil {
		t.Fatal(err)
	}
	if err := writeWorkerCancel(workerCancelPath(dir, id)); err != nil {
		t.Fatal(err)
	}
	mkText(t, workerInfPath(dir, id), "[Setup]")
	return []string{
		workerSpecFilePath(dir, id), workerStatePath(dir, id), workerCancelPath(dir, id), workerInfPath(dir, id),
	}
}

func TestElevatedInstallLeavesNoWorkerFilesBehind(t *testing.T) {
	cases := []struct {
		name   string
		act    func(t *testing.T, dest string, ws workerSpec) workerState
		status Status
		silent bool
		keeps  bool
	}{
		{"completed", func(t *testing.T, dest string, _ workerSpec) workerState {
			mkFile(t, filepath.Join(dest, "Game.exe"), 8192)
			return workerState{}
		}, StatusCompleted, false, false},
		{"installer failure code", func(t *testing.T, _ string, ws workerSpec) workerState {
			if err := os.WriteFile(ws.LogPath, utf16Log("Rolling back changes"), 0o600); err != nil {
				t.Errorf("write log: %v", err)
			}
			return workerState{Code: 1}
		}, StatusFailed, false, false},
		{"error text from the worker", func(*testing.T, string, workerSpec) workerState {
			return workerState{Error: "installer job could not start"}
		}, StatusFailed, false, false},
		{"worker gone without a result", func(*testing.T, string, workerSpec) workerState {
			return workerState{}
		}, StatusFailed, true, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newRig(t)
			dest := silentSource(t, r, rgInnoMarker)
			worker := &rgWorker{t: t, silent: tc.silent, act: func(ws workerSpec) workerState { return tc.act(t, dest, ws) }}
			worker.install()
			r.setRunner(elevatingRunner{})
			item, err := r.s.Start("d1", StartOptions{Destination: dest})
			if err != nil {
				t.Fatal(err)
			}
			if got := r.settle(item.ID); got.Status != tc.status {
				t.Fatalf("status = %s (%q), want %s", got.Status, got.Error, tc.status)
			}

			left := workerFilesOf(r)
			switch {
			case tc.keeps && len(left) == 0:
				t.Fatal("a worker that never confirmed it stopped lost its files, the launcher can no longer tell it is still running")
			case !tc.keeps && len(left) != 0:
				t.Fatalf("worker files after the launcher consumed the final state = %v, want none", left)
			}

			if tc.status == StatusFailed {
				if err := r.s.Dismiss(item.ID); err != nil {
					t.Fatal(err)
				}
			}
			if files := r.stateFiles(); len(files) != 0 {
				t.Fatalf("state directory after the install is settled and dismissed holds %v, want nothing", files)
			}
		})
	}
}

func TestDismissRemovesOnlyFilesOfItsOwnInstall(t *testing.T) {
	r := newRig(t)
	r.add(Installation{ID: "aa", Name: "A", Status: StatusFailed, Error: "boom"})
	r.add(Installation{ID: "bb", Name: "B", Status: StatusFailed, Error: "boom"})
	own := seedWorkerFiles(t, r.dir, "aa", workerState{Done: true, Code: 1})
	other := seedWorkerFiles(t, r.dir, "bb", workerState{Done: true, Code: 1})
	ownLog := r.s.installerLogPath("aa")
	otherLog := r.s.installerLogPath("bb")
	mkText(t, ownLog, "log a")
	mkText(t, otherLog, "log b")

	if err := r.s.Dismiss("aa"); err != nil {
		t.Fatal(err)
	}

	for _, path := range append(own, ownLog) {
		if exists(path) {
			t.Fatalf("%s survived the Dismiss of its install", path)
		}
	}
	for _, path := range append(other, otherLog) {
		if !exists(path) {
			t.Fatalf("%s of another install was removed by the Dismiss", path)
		}
	}
}

func TestDismissKeepsTheRecordWhenItsFilesCannotBeRemoved(t *testing.T) {
	r := newRig(t)
	r.add(Installation{ID: "cc", Name: "C", Status: StatusFailed, Error: "boom"})
	stuck := workerStatePath(r.dir, "cc")
	mkFile(t, filepath.Join(stuck, "inside.bin"), 8)

	err := r.s.Dismiss("cc")
	if err == nil {
		t.Fatal("Dismiss reported success though the worker state could not be removed")
	}
	if got := r.s.List(); len(got) != 1 || got[0].ID != "cc" {
		t.Fatalf("records after a refused Dismiss = %+v, want the record kept so the user can retry", got)
	}
	if got := r.disk(); len(got) != 1 {
		t.Fatalf("installations.json after a refused Dismiss = %+v", got)
	}
}

func TestRetryClearsTheFilesOfThePreviousRun(t *testing.T) {
	r := newRig(t)
	item := failedSilentInstall(t, r)
	stale := seedWorkerFiles(t, r.dir, item.ID, workerState{Done: true, Code: 1, Run: "old"})
	run := newRgRunner(t, rgStep{wait: true})
	r.setRunner(run)

	if err := r.s.Retry(item.ID); err != nil {
		t.Fatal(err)
	}
	run.entered(0)

	for _, path := range stale {
		if exists(path) {
			t.Fatalf("%s of the previous run is still there while the retry runs", path)
		}
	}
	if err := r.s.Cancel(item.ID); err != nil {
		t.Fatal(err)
	}
	r.settle(item.ID)
}

func TestRetryFailsWhenTheFilesOfThePreviousRunCannotBeCleared(t *testing.T) {
	r := newRig(t)
	item := failedSilentInstall(t, r)
	failed := r.get(item.ID)
	stuck := workerStatePath(r.dir, item.ID)
	mkFile(t, filepath.Join(stuck, "inside.bin"), 8)
	run := newRgRunner(t, rgStep{})
	r.setRunner(run)

	if err := r.s.Retry(item.ID); err == nil {
		t.Fatal("Retry reported success though the state of the previous run is still in place")
	}
	got := r.get(item.ID)
	if got.Status != StatusFailed || got.Error != failed.Error {
		t.Fatalf("record after the refused Retry: status %s error %q, want it left failed with %q", got.Status, got.Error, failed.Error)
	}
	if n := len(run.calls()); n != 0 {
		t.Fatalf("installer started %d times after a refused Retry", n)
	}
	if disk := r.diskItem(item.ID); disk.Status != StatusFailed {
		t.Fatalf("installations.json holds %s after a refused Retry", disk.Status)
	}
}

// Лаунчер мог умереть между Retry и первой записью нового воркера: старый Done в
// файле состояния не должен выдаваться за итог нового прогона.
func TestStaleWorkerResultIsNotTrustedAfterARetryInterruptedByARestart(t *testing.T) {
	r := newRig(t)
	item := failedSilentInstall(t, r)
	seedWorkerFiles(t, r.dir, item.ID, workerState{Done: true, Code: 1, Run: "old"})
	run := newRgRunner(t, rgStep{wait: true})
	r.setRunner(run)
	if err := r.s.Retry(item.ID); err != nil {
		t.Fatal(err)
	}
	run.entered(0)

	r.restart()

	got := r.get(item.ID)
	if got.Status != StatusInterrupted {
		t.Fatalf("status after a restart in the middle of a retry = %s (%q), want interrupted: the previous run's result must not be resumed", got.Status, got.Error)
	}
}

func TestResumedInstallRemovesTheWorkerFilesItConsumed(t *testing.T) {
	cases := []struct {
		name   string
		state  workerState
		status Status
	}{
		{"worker finished", workerState{Done: true}, StatusCompleted},
		{"worker reported an error", workerState{Done: true, Error: "installer job could not start"}, StatusFailed},
		{"worker confirmed the cancel", workerState{Done: true, Cancelled: true, Error: "context canceled"}, StatusCancelled},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newRig(t)
			item := r.addResumable("res9")
			files := seedWorkerFiles(t, r.dir, item.ID, tc.state)

			r.s.finishResumed(r.s.ctx, item.ID, tc.state)

			if got := r.get(item.ID); got.Status != tc.status {
				t.Fatalf("status = %s (%q), want %s", got.Status, got.Error, tc.status)
			}
			for _, path := range files {
				if exists(path) {
					t.Fatalf("%s survived the completion of the resumed install", path)
				}
			}
		})
	}
}

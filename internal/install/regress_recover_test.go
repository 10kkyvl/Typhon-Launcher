package install

import (
	"path/filepath"
	"testing"
	"time"
)

// seedRecords подменяет installations.json тем, что осталось бы после аварии:
// сервис остановлен, записи лежат на диске в произвольных состояниях.
func (r *rig) seedRecords(items ...Installation) {
	r.t.Helper()
	r.seedWith(nil, items...)
}

// seedWith даёт подготовить диск (файлы воркера, остатки распаковки) до того,
// как сервис впервые увидит записи: именно в этот момент он решает их судьбу.
func (r *rig) seedWith(prepare func(), items ...Installation) {
	r.t.Helper()
	r.shutdown()
	if prepare != nil {
		prepare()
	}
	if err := newStore(r.dir).save(items); err != nil {
		r.t.Fatalf("seed installations.json: %v", err)
	}
	r.boot()
}

var transientStatuses = []Status{StatusPending, StatusPreparing, StatusInstalling, StatusExtracting, StatusVerifying}

func TestStartupInterruptsEveryTransientRecord(t *testing.T) {
	kinds := []struct {
		name    string
		partial bool
		worker  bool
		item    func(dest string) Installation
	}{
		{name: "zip with a half-extracted partial", partial: true, item: func(dest string) Installation {
			return Installation{Type: TypeArchiveZip, Destination: dest, Mode: ModeCopy}
		}},
		{name: "portable copy with a half-copied partial", partial: true, item: func(dest string) Installation {
			return Installation{Type: TypePortable, Destination: dest, Mode: ModeCopy, ContentRoot: dest + "-source"}
		}},
		{name: "silent installer whose worker left no state", item: func(dest string) Installation {
			return Installation{Type: TypeExeInstaller, Engine: EngineInno, Silent: true, Destination: dest}
		}},
		{name: "silent installer whose worker died without a result", worker: true, item: func(dest string) Installation {
			return Installation{Type: TypeExeInstaller, Engine: EngineInno, Silent: true, Destination: dest}
		}},
		{name: "interactive installer", item: func(string) Installation {
			return Installation{Type: TypeExeInstaller, Engine: EngineUnknown}
		}},
		{name: "silent msi", item: func(dest string) Installation {
			return Installation{Type: TypeMsiInstaller, Engine: EngineMsi, Silent: true, Destination: dest}
		}},
	}
	for _, kind := range kinds {
		for _, status := range transientStatuses {
			t.Run(kind.name+"/"+string(status), func(t *testing.T) {
				r := newRig(t)
				dest := filepath.Join(t.TempDir(), "Game")
				item := kind.item(dest)
				item.ID, item.DownloadID, item.Name, item.Status, item.StartedAt = "crash1", "d1", "Game", status, time.Now()
				if kind.partial {
					mkFile(t, filepath.Join(dest+partialSuffix, "chunk.bin"), 16)
				}
				r.seedWith(func() {
					if !kind.worker {
						return
					}
					deadWorker := workerState{PID: 999999999}
					if err := writeWorkerState(workerStatePath(r.dir, item.ID), deadWorker); err != nil {
						t.Fatalf("write worker state: %v", err)
					}
				}, item)

				got := r.get(item.ID)
				if got.Status != StatusInterrupted || got.Error != errInterrupted.Error() || uiCode(got.Error) != "install.interrupted" {
					t.Fatalf("status %s error %q, want interrupted", got.Status, got.Error)
				}
				if disk := r.diskItem(item.ID); disk.Status != StatusInterrupted {
					t.Fatalf("installations.json holds %s, want interrupted", disk.Status)
				}
				if kind.partial {
					waitFor(t, "the partial to be swept", func() bool { return !exists(dest + partialSuffix) })
				}
				if games := r.reg.registered(); len(games) != 0 {
					t.Fatalf("an interrupted record registered %+v", games)
				}
				r.restart()
				if again := r.get(item.ID); viewOf(again) != viewOf(got) {
					t.Fatalf("a second restart changed the interrupted record: %+v -> %+v", viewOf(got), viewOf(again))
				}
			})
		}
	}
}

func TestStartupLeavesSettledRecordsAlone(t *testing.T) {
	done := time.Now()
	records := []Installation{
		{ID: "a", Name: "Waiting", Type: TypeExeInstaller, Status: StatusWaitingForUser, Destination: filepath.Join(t.TempDir(), "A"), Candidates: []Candidate{{Path: "a.exe", Score: 70}}},
		{ID: "b", Name: "Done", Type: TypeArchiveZip, Status: StatusCompleted, GameID: "game-9", CompletedAt: &done},
		{ID: "c", Name: "Failed", Type: TypeArchiveZip, Status: StatusFailed, Error: "typhon:install.installer_failed: boom"},
		{ID: "d", Name: "Cancelled", Type: TypeArchiveZip, Status: StatusCancelled},
		{ID: "e", Name: "Interrupted", Type: TypeArchiveZip, Status: StatusInterrupted, Error: errInterrupted.Error()},
	}
	r := newRig(t)
	r.seedRecords(records...)
	for _, want := range records {
		got := r.get(want.ID)
		if got.Status != want.Status || got.Error != want.Error || got.GameID != want.GameID || len(got.Candidates) != len(want.Candidates) {
			t.Fatalf("record %s changed on startup: %+v", want.ID, viewOf(got))
		}
	}
	if n := len(r.s.List()); n != len(records) {
		t.Fatalf("records = %d, want %d", n, len(records))
	}
}

// Прерванная при аварии установка обязана снова запускаться из той же записи:
// загрузка на месте, поэтому повтор доводит её до конца без новой записи.
func TestInterruptedZipInstallRetriesToCompletion(t *testing.T) {
	r := newRig(t)
	dest := archiveSource(t, r, "game.zip", func(path string) { writeZip(t, path, gameZipEntries()) })
	mkFile(t, filepath.Join(dest+partialSuffix, "Game.exe"), 16)
	r.seedRecords(Installation{
		ID: "crash2", DownloadID: "d1", Name: "Game", Type: TypeArchiveZip, Status: StatusExtracting,
		Destination: dest, Owned: true, StartedAt: time.Now(),
	})
	if got := r.get("crash2"); got.Status != StatusInterrupted {
		t.Fatalf("status = %s, want interrupted", got.Status)
	}
	waitFor(t, "the partial to be swept", func() bool { return !exists(dest + partialSuffix) })

	if err := r.s.Retry("crash2"); err != nil {
		t.Fatalf("Retry: %v", err)
	}
	done := r.settle("crash2")
	if done.Status != StatusCompleted || done.Error != "" || done.Executable != filepath.Join(dest, "Game.exe") {
		t.Fatalf("after retry: %+v", viewOf(done))
	}
	if n := len(r.s.List()); n != 1 {
		t.Fatalf("records = %d, want the retry to reuse the record", n)
	}
	r.assertDurable("crash2")
}

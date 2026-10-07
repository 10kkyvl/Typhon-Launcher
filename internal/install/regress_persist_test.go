package install

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// add кладёт запись и в память, и на диск: дальше тест блокирует хранилище и
// смотрит, во что превращается память, когда диск отказывает.
func (r *rig) add(item Installation) {
	r.t.Helper()
	if item.StartedAt.IsZero() {
		item.StartedAt = time.Now().Truncate(time.Second)
	}
	r.s.mu.Lock()
	r.s.items = append(r.s.items, &item)
	err := r.s.persistLocked()
	r.s.mu.Unlock()
	if err != nil {
		r.t.Fatalf("persist %s: %v", item.ID, err)
	}
}

func wantPersistError(t *testing.T, err error) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), errPersistPrefix) {
		t.Fatalf("error = %v, want the %q wrapper", err, errPersistPrefix)
	}
}

type persistCase struct {
	name string
	item Installation
	do   func(r *rig, id string) error
}

func persistCases() []persistCase {
	return []persistCase{
		{"setStatus", Installation{ID: "p1", Name: "Game", Status: StatusPending}, func(r *rig, id string) error {
			return r.s.setStatus(id, StatusPreparing)
		}},
		{"setInstallerVerifying", Installation{ID: "p1", Name: "Game", Status: StatusInstalling}, func(r *rig, id string) error {
			return r.s.setInstallerVerifying(id)
		}},
		{"waitForUser", Installation{ID: "p1", Name: "Game", Status: StatusInstalling}, func(r *rig, id string) error {
			return r.s.waitForUser(id, []Candidate{{Path: "a.exe", Score: 70}})
		}},
		{"setExecutable", Installation{ID: "p1", Name: "Game", Status: StatusVerifying}, func(r *rig, id string) error {
			return r.s.setExecutable(id, "a.exe", []Candidate{{Path: "a.exe", Score: 70}})
		}},
		{"setDestination", Installation{ID: "p1", Name: "Game", Status: StatusInstalling}, func(r *rig, id string) error {
			return r.s.setDestination(id, filepath.Join(r.games, "Game"))
		}},
		{"forceDestination", Installation{ID: "p1", Name: "Game", Status: StatusInstalling, Destination: "old"}, func(r *rig, id string) error {
			return r.s.forceDestination(id, filepath.Join(r.games, "Other"))
		}},
		{"setRemoval", Installation{ID: "p1", Name: "Game", Status: StatusInstalling, UninstallUnknown: true}, func(r *rig, id string) error {
			return r.s.setRemoval(id, filepath.Join(r.games, "Game"), fsSnapshot{}, nil, "Game")
		}},
		{"rememberInstallerDestination", Installation{ID: "p1", Name: "Game", Status: StatusPreparing}, func(r *rig, id string) error {
			return r.s.rememberInstallerDestination(id, filepath.Join(r.games, "Fresh"))
		}},
		{"markResumedOwnership", Installation{ID: "p1", Name: "Game", Status: StatusInstalling, Owned: true, Destination: "x"}, func(r *rig, id string) error {
			return r.s.markResumedOwnership(id)
		}},
		{"beginChainStep", Installation{ID: "p1", Name: "Game", Status: StatusInstalling, ChainStep: 1}, func(r *rig, id string) error {
			return r.s.beginChainStep(id, 2)
		}},
	}
}

// requireUnwritableDir пропускает такие тесты на Windows целиком, поэтому откат
// памяти при отказе диска там не проверялся вовсе (инвариант 4). Здесь диск
// отказывает подменой файла каталогом — и на Windows, и на POSIX.
func TestPersistFailureRollsBackMemory(t *testing.T) {
	for _, tc := range persistCases() {
		t.Run(tc.name, func(t *testing.T) {
			r := newRig(t)
			r.add(tc.item)
			before := r.get(tc.item.ID)
			unblock := r.blockStore()

			wantPersistError(t, tc.do(r, tc.item.ID))
			if after := r.get(tc.item.ID); !reflect.DeepEqual(before, after) {
				t.Fatalf("memory was not rolled back:\n before %+v\n after  %+v", before, after)
			}

			unblock()
			if err := tc.do(r, tc.item.ID); err != nil {
				t.Fatalf("the same step after the disk recovered: %v", err)
			}
			if disk, mem := r.diskItem(tc.item.ID), r.get(tc.item.ID); !reflect.DeepEqual(viewOf(disk), viewOf(mem)) {
				t.Fatalf("installations.json %+v differs from memory %+v after recovery", viewOf(disk), viewOf(mem))
			}
		})
	}
}

func TestStartRefusedWhenStateCannotBeSaved(t *testing.T) {
	r := newRig(t)
	root := t.TempDir()
	portableSource(t, root, "Game")
	r.download("d1", "Game", root)
	dest := filepath.Join(t.TempDir(), "Game")
	unblock := r.blockStore()

	_, err := r.s.Start("d1", StartOptions{Destination: dest, Mode: ModeCopy})
	wantPersistError(t, err)
	if got := r.s.List(); len(got) != 0 {
		t.Fatalf("a Start whose record was never saved left %+v in memory", got)
	}
	if exists(dest) || exists(dest+partialSuffix) {
		t.Fatalf("a Start whose record was never saved touched %s", dest)
	}

	unblock()
	item, err := r.s.Start("d1", StartOptions{Destination: dest, Mode: ModeCopy})
	if err != nil {
		t.Fatalf("Start after the disk recovered: %v", err)
	}
	if got := r.settle(item.ID); got.Status != StatusCompleted {
		t.Fatalf("status = %s (%q)", got.Status, got.Error)
	}
}

func TestDismissKeepsTheRecordWhenStateCannotBeSaved(t *testing.T) {
	r := newRig(t)
	r.add(Installation{ID: "a", Name: "a", Status: StatusFailed, Error: "boom"})
	before := r.get("a")
	unblock := r.blockStore()

	wantPersistError(t, r.s.Dismiss("a"))
	if after := r.get("a"); !reflect.DeepEqual(before, after) {
		t.Fatalf("a Dismiss that could not be saved changed the record:\n before %+v\n after  %+v", before, after)
	}

	unblock()
	if err := r.s.Dismiss("a"); err != nil {
		t.Fatalf("Dismiss after the disk recovered: %v", err)
	}
	if got := r.disk(); len(got) != 0 {
		t.Fatalf("installations.json still holds %+v", got)
	}
	if got := r.s.List(); len(got) != 0 {
		t.Fatalf("List still holds %+v", got)
	}
}

func TestRetryRollsBackWhenStateCannotBeSaved(t *testing.T) {
	r := newRig(t)
	dest := silentSource(t, r, rgInnoMarker)
	failing := newRgRunner(t, rgStep{code: 1})
	r.setRunner(failing)
	item, err := r.s.Start("d1", StartOptions{Destination: dest})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	before := r.settle(item.ID)
	if before.Status != StatusFailed {
		t.Fatalf("status = %s", before.Status)
	}
	unblock := r.blockStore()

	wantPersistError(t, r.s.Retry(item.ID))
	if after := r.get(item.ID); !reflect.DeepEqual(before, after) {
		t.Fatalf("a Retry that could not be saved changed the record:\n before %+v\n after  %+v", before, after)
	}
	if n := len(failing.calls()); n != 1 {
		t.Fatalf("installer runs = %d, want the failed Retry to start nothing", n)
	}

	unblock()
	r.setRunner(newRgRunner(t, rgStep{act: installGame(t, dest)}))
	if err := r.s.Retry(item.ID); err != nil {
		t.Fatalf("Retry after the disk recovered: %v", err)
	}
	if got := r.settle(item.ID); got.Status != StatusCompleted {
		t.Fatalf("status = %s (%q)", got.Status, got.Error)
	}
}

func TestConfirmExecutableRollsBackWhenStateCannotBeSaved(t *testing.T) {
	r := newRig(t)
	installed := interactiveSource(t, r)
	r.setRunner(newRgRunner(t, rgStep{act: func(runSpec) {
		mkFile(t, filepath.Join(installed, "MyGame.exe"), 512<<10)
		mkFile(t, filepath.Join(installed, "Other.exe"), 8192)
	}}))
	item, err := r.s.Start("d1", StartOptions{})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	before := r.settle(item.ID)
	if before.Status != StatusWaitingForUser {
		t.Fatalf("status = %s (%q)", before.Status, before.Error)
	}
	unblock := r.blockStore()

	wantPersistError(t, r.s.ConfirmExecutable(item.ID, filepath.Join(installed, "Other.exe")))
	if after := r.get(item.ID); !reflect.DeepEqual(before, after) {
		t.Fatalf("a Confirm that could not be saved changed the record:\n before %+v\n after  %+v", before, after)
	}
	if games := r.reg.registered(); len(games) != 0 {
		t.Fatalf("a Confirm that could not be saved registered %+v", games)
	}

	unblock()
	if err := r.s.ConfirmExecutable(item.ID, filepath.Join(installed, "Other.exe")); err != nil {
		t.Fatalf("Confirm after the disk recovered: %v", err)
	}
	if got := r.get(item.ID); got.Status != StatusCompleted || got.Executable != filepath.Join(installed, "Other.exe") {
		t.Fatalf("after confirm: %+v", viewOf(got))
	}
}

// Отмена записи без job (после перезапуска, воркер мёртв) не может притвориться
// удавшейся: диск отказал, значит UI получает ошибку и запись в состоянии «провал».
func TestCancelWithoutJobReportsPersistFailure(t *testing.T) {
	r := newRig(t)
	r.add(Installation{ID: "p1", Name: "Game", Type: TypeExeInstaller, Status: StatusInstalling})
	unblock := r.blockStore()

	err := r.s.Cancel("p1")
	wantPersistError(t, err)
	got := r.get("p1")
	if got.Status != StatusFailed || !strings.Contains(got.Error, errPersistPrefix) {
		t.Fatalf("record after a Cancel that could not be saved: status %s error %q", got.Status, got.Error)
	}

	unblock()
	if err := r.s.Dismiss("p1"); err != nil {
		t.Fatalf("Dismiss after the disk recovered: %v", err)
	}
}

func TestShutdownReportsPersistFailure(t *testing.T) {
	r := newRig(t)
	r.add(Installation{ID: "p1", Name: "Game", Status: StatusFailed})
	unblock := r.blockStore()
	if err := r.s.ServiceShutdown(); err == nil {
		t.Fatal("ServiceShutdown returned nil although the state could not be written")
	}
	unblock()
}

// Каталог состояния, который нельзя создать, — отказ запуска, а не работа «в никуда».
func TestStartupFailsWhenStateDirIsNotADirectory(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	s := mustServiceAt(t, filepath.Join(blocker, "state"))
	if err := s.ServiceStartup(context.Background(), application.ServiceOptions{}); err == nil {
		t.Fatal("startup succeeded over a state directory that cannot exist")
	}
	if base, err := s.baseContext(); err == nil && base.Err() == nil {
		t.Fatal("a service whose startup failed holds a live context: its jobs would outlive a launcher that never started")
	}
}

func TestJobFailsLoudlyWhenStateStopsBeingSaved(t *testing.T) {
	r := newRig(t)
	dest := silentSource(t, r, rgInnoMarker)
	r.setRunner(newRgRunner(t, rgStep{act: func(runSpec) {
		installGame(t, dest)(runSpec{})
		r.blockStore()
	}}))
	item, err := r.s.Start("d1", StartOptions{Destination: dest})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	got := r.settle(item.ID)
	if got.Status != StatusFailed || !strings.Contains(got.Error, errPersistPrefix) {
		t.Fatalf("status %s error %q, want a failure that carries the save error to the UI", got.Status, got.Error)
	}
	if games := r.reg.registered(); len(games) != 0 {
		t.Fatalf("registered %+v although the install state could not be saved", games)
	}
}

package selfupdate

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
)

func stubCurrentExecutable(t *testing.T, exe string) {
	t.Helper()
	restore := currentExecutable
	currentExecutable = func() (string, error) { return exe, nil }
	t.Cleanup(func() { currentExecutable = restore })
}

func rollbackService(t *testing.T, f rollbackFixture, current string) *Service {
	t.Helper()
	stubRollbackSupported(t, nil)
	stubCurrentExecutable(t, f.target)
	return &Service{dir: f.configDir, notes: mustNotesStore(t, f.configDir), store: mustStore(t, f.configDir), currentVersion: current}
}

func TestGetRollback(t *testing.T) {
	f := newRollbackFixture(t)
	s := rollbackService(t, f, "1.3.0")

	if got := s.GetRollback(); !got.Available || got.Version != "1.2.0" || got.Reason != "" || got.ReasonCode != "" {
		t.Fatalf("GetRollback = %+v", got)
	}
	if err := dropPrevious(f.configDir); err != nil {
		t.Fatal(err)
	}
	got := s.GetRollback()
	if got.Available || got.ReasonCode != "selfupdate.rollback_none" || got.Reason == "" {
		t.Fatalf("GetRollback with nothing saved = %+v", got)
	}
}

func TestApplyRollbackHandsOverToTheWorker(t *testing.T) {
	f := newRollbackFixture(t)
	s := rollbackService(t, f, "1.3.0")
	started := captureWorkerStarts(t)

	if err := s.ApplyRollback("ru"); err != nil {
		t.Fatalf("ApplyRollback: %v", err)
	}

	if len(started.specs) != 1 {
		t.Fatalf("workers started = %d", len(started.specs))
	}
	spec, err := readUpdateSpec(started.specs[0])
	if err != nil {
		t.Fatal(err)
	}
	want := updateSpec{
		Kind:         specKindRollback,
		Language:     "ru",
		InstallDir:   filepath.Dir(f.target),
		ParentPID:    os.Getpid(),
		RelaunchPath: f.target,
		Version:      "1.2.0",
		FromVersion:  "1.3.0",
	}
	if spec != want {
		t.Fatalf("spec = %+v, want %+v", spec, want)
	}
	workerDir := mustWorkerDir(t, f.configDir)
	if filepath.Dir(started.workers[0]) != workerDir {
		t.Fatalf("worker at %q, want it inside %q: it must not run from the file it replaces", started.workers[0], workerDir)
	}
	if got := readTestFile(t, started.workers[0]); got != f.newExe {
		t.Fatalf("worker copy = %q, want a copy of the running launcher", got)
	}
	if !s.busy || !s.rollbackPending {
		t.Fatalf("busy = %v, rollbackPending = %v: the service is busy until the process is gone", s.busy, s.rollbackPending)
	}
	if err := s.ApplyRollback("ru"); !errors.Is(err, ErrBusy) {
		t.Fatalf("second ApplyRollback = %v, want ErrBusy", err)
	}
	if err := s.ServiceShutdown(); err != nil {
		t.Fatal(err)
	}
}

func TestApplyRollbackRefusalsReleaseTheService(t *testing.T) {
	errStart := errors.New("process creation refused")
	cases := []struct {
		name      string
		supported error
		damage    func(t *testing.T, f rollbackFixture)
		start     error
		want      error
	}{
		{name: "nothing saved", want: ErrNoPrevious, damage: func(t *testing.T, f rollbackFixture) {
			if err := dropPrevious(f.configDir); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "platform without rollback", supported: ErrRollbackUnsupported, want: ErrRollbackUnsupported},
		{name: "worker cannot start", start: errStart, want: errStart},
		{name: "damaged snapshot", want: ErrPreviousBroken, damage: func(t *testing.T, f rollbackFixture) {
			dir := mustPreviousDir(t, f.configDir)
			writeTestFile(t, filepath.Join(dir, previousExeName), []byte("x"))
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newRollbackFixture(t)
			s := rollbackService(t, f, "1.3.0")
			stubRollbackSupported(t, tc.supported)
			started := captureWorkerStarts(t)
			if tc.start != nil {
				startWorker = func(string, string) error { return tc.start }
			}
			if tc.damage != nil {
				tc.damage(t, f)
			}

			err := s.ApplyRollback("en")
			if !errors.Is(err, tc.want) {
				t.Fatalf("ApplyRollback = %v, want %v", err, tc.want)
			}
			if s.busy || s.rollbackPending {
				t.Fatalf("busy = %v, rollbackPending = %v after a refusal", s.busy, s.rollbackPending)
			}
			if tc.start == nil && len(started.specs) != 0 {
				t.Fatalf("a refused rollback started %d workers", len(started.specs))
			}
			if got := readTestFile(t, f.target); got != f.newExe {
				t.Fatalf("launcher = %q", got)
			}
		})
	}
}

func TestApplyRollbackWhileAnotherOperationRuns(t *testing.T) {
	f := newRollbackFixture(t)
	s := rollbackService(t, f, "1.3.0")
	started := captureWorkerStarts(t)
	s.busy = true

	if err := s.ApplyRollback("en"); !errors.Is(err, ErrBusy) {
		t.Fatalf("ApplyRollback = %v, want ErrBusy", err)
	}
	if len(started.specs) != 0 {
		t.Fatal("a worker was started while the service was busy")
	}
}

func TestALauncherThatDoesNotExitAfterARollbackFreesTheServiceAndSaysSo(t *testing.T) {
	restartBudget(t, time.Millisecond, time.Millisecond)
	f := newRollbackFixture(t)
	s := rollbackService(t, f, "1.3.0")
	started := captureWorkerStarts(t)
	missed := make(chan struct{}, 2)
	s.onRestartMissed = func() { missed <- struct{}{} }
	t.Cleanup(func() {
		if err := s.ServiceShutdown(); err != nil {
			t.Errorf("ServiceShutdown: %v", err)
		}
	})

	if err := s.ApplyRollback("en"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-missed:
	case <-time.After(5 * time.Second):
		t.Fatal("the service stayed busy after the worker gave up on the launcher")
	}

	s.mu.Lock()
	busy, pending := s.busy, s.rollbackPending
	s.mu.Unlock()
	if busy || pending {
		t.Fatalf("busy = %v, rollbackPending = %v", busy, pending)
	}
	if st := s.GetStatus(); st.ErrorCode != "rollback" || !strings.HasPrefix(st.Error, "typhon:selfupdate.parent_still_running:") {
		t.Fatalf("status = %+v, want the failure reported with the code the UI translates", st)
	}
	if err := s.ApplyRollback("en"); err != nil {
		t.Fatalf("retry: %v", err)
	}
	if len(started.specs) != 2 {
		t.Fatalf("workers started = %d, want a second start for the retry", len(started.specs))
	}
}

func TestServiceStartupDropsTheSnapshotOfTheRunningVersion(t *testing.T) {
	cases := []struct {
		name    string
		current string
		kept    bool
	}{
		{"a rollback finished and left its snapshot", "1.2.0", false},
		{"same version written another way", "v1.2.0", false},
		{"the update this snapshot belongs to", "1.3.0", true},
		{"another version", "1.4.0", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newRollbackFixture(t)
			stubCurrentExecutable(t, f.target)
			restartedService(t, f.configDir, tc.current)

			_, err := loadPrevious(f.configDir)
			if tc.kept && err != nil {
				t.Fatalf("snapshot was dropped: %v", err)
			}
			if !tc.kept && !errors.Is(err, ErrNoPrevious) {
				t.Fatalf("loadPrevious = %v, want the snapshot gone", err)
			}
		})
	}
}

func TestServiceStartupClearsSwapLeftoversBesideTheLauncher(t *testing.T) {
	f := newRollbackFixture(t)
	stubCurrentExecutable(t, f.target)
	stale := filepath.Join(filepath.Dir(f.target), ".tmp-4711")
	writeTestFile(t, stale, []byte("half a launcher"))
	ageFile(t, stale, 3*swapLeftoverAge)

	restartedService(t, f.configDir, "1.3.0")

	if _, err := os.Stat(stale); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("leftover survived the start: %v", err)
	}
	if got := readTestFile(t, f.target); got != f.newExe {
		t.Fatalf("launcher = %q", got)
	}
}

func TestCleanupCacheKeepsTheRollbackAndTheStartMarker(t *testing.T) {
	f := newRollbackFixture(t)
	stubCurrentExecutable(t, f.target)
	stubInstanceLock(t, false)
	BeginStart(f.configDir, "1.3.0")
	cache := mustCacheDir(t, f.configDir)
	writeTestFile(t, filepath.Join(cache, "1.0.0", "setup.exe"), []byte("old installer"))
	writeTestFile(t, filepath.Join(cache, "stray.bin"), []byte("stray"))

	restartedService(t, f.configDir, "1.3.0")

	var left []string
	entries, err := os.ReadDir(cache)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		left = append(left, e.Name())
	}
	sort.Strings(left)
	for _, keep := range []string{previousDirName, previousRecordName, startupMarkerName} {
		found := false
		for _, name := range left {
			found = found || name == keep
		}
		if !found {
			t.Fatalf("cleanupCache removed %s; left: %v", keep, left)
		}
	}
	for _, gone := range []string{"1.0.0", "stray.bin"} {
		for _, name := range left {
			if name == gone {
				t.Fatalf("cleanupCache kept %s; left: %v", gone, left)
			}
		}
	}
	if _, err := loadPrevious(f.configDir); err != nil {
		t.Fatalf("snapshot after cleanup: %v", err)
	}
	dir := mustPreviousDir(t, f.configDir)
	if got := readTestFile(t, filepath.Join(dir, previousExeName)); got != f.oldExe {
		t.Fatalf("saved launcher after cleanup = %q", got)
	}
}

func TestSkippedVersionIsNotOfferedUntilANewerOneComes(t *testing.T) {
	cases := []struct {
		name      string
		skipped   string
		published string
		offered   bool
	}{
		{"nothing skipped", "", "1.3.0", true},
		{"the skipped version", "1.3.0", "1.3.0", false},
		{"an older release than the skipped one", "1.3.0", "1.2.5", false},
		{"a newer release", "1.3.0", "1.3.1", true},
		{"a much newer release", "1.3.0", "1.10.0", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stand := newReleaseStand(t, tc.published, []byte("installer"))
			dir := t.TempDir()
			s := standService(t, dir, stand, "1.2.0")
			s.skippedVersion = tc.skipped

			status, err := s.CheckForUpdate(context.Background())
			if err != nil {
				t.Fatalf("CheckForUpdate: %v", err)
			}
			saved, err := mustStore(t, dir).Load()
			if err != nil {
				t.Fatal(err)
			}
			if tc.offered {
				if status.State != StateAvailable || status.AvailableVersion != tc.published {
					t.Fatalf("status = %+v, want %s offered", status, tc.published)
				}
			} else {
				if status.State != StateIdle || status.AvailableVersion != "" {
					t.Fatalf("status = %+v, want nothing offered", status)
				}
				if _, err := s.DownloadUpdate(context.Background()); !errors.Is(err, errNoUpdateChecked) {
					t.Fatalf("DownloadUpdate = %v: a skipped version must not be downloadable", err)
				}
			}
			if saved.SkippedVersion != tc.skipped {
				t.Fatalf("saved skipped version = %q, want %q: a check must not forget it", saved.SkippedVersion, tc.skipped)
			}
		})
	}
}

func TestSkippedVersionSurvivesDownloadDismissAndRestart(t *testing.T) {
	stand := newReleaseStand(t, "1.3.1", []byte("installer"))
	dir := t.TempDir()
	if err := mustStore(t, dir).Save(stored{SkippedVersion: "1.3.0"}); err != nil {
		t.Fatal(err)
	}

	s := standService(t, dir, stand, "1.2.0")
	if err := s.ServiceStartup(context.Background(), application.ServiceOptions{}); err != nil {
		t.Fatalf("ServiceStartup: %v", err)
	}
	t.Cleanup(func() {
		if err := s.ServiceShutdown(); err != nil {
			t.Errorf("ServiceShutdown: %v", err)
		}
	})
	if got := s.skipped(); got != "1.3.0" {
		t.Fatalf("skipped version after startup = %q", got)
	}

	for _, step := range []struct {
		name string
		run  func() error
	}{
		{"check", func() error { _, err := s.CheckForUpdate(context.Background()); return err }},
		{"download", func() error { _, err := s.DownloadUpdate(context.Background()); return err }},
		{"dismiss", s.DismissUpdate},
	} {
		if err := step.run(); err != nil {
			t.Fatalf("%s: %v", step.name, err)
		}
		saved, err := mustStore(t, dir).Load()
		if err != nil {
			t.Fatal(err)
		}
		if saved.SkippedVersion != "1.3.0" {
			t.Fatalf("after %s the saved skipped version is %q", step.name, saved.SkippedVersion)
		}
	}
}

func TestApplyUpdateTellsTheWorkerWhichVersionItLeaves(t *testing.T) {
	dir := t.TempDir()
	readyPath := filepath.Join(dir, "selfupdate", "1.3.0", "setup.exe")
	writeTestFile(t, readyPath, []byte("installer"))
	s := &Service{dir: dir, notes: mustNotesStore(t, dir), store: mustStore(t, dir), currentVersion: "1.2.0", readyPath: readyPath}
	s.status = Status{State: StateReady, CurrentVersion: "1.2.0", AvailableVersion: "1.3.0"}
	started := captureWorkerStarts(t)

	if err := s.ApplyUpdate("en"); err != nil {
		t.Fatalf("ApplyUpdate: %v", err)
	}
	spec, err := readUpdateSpec(started.specs[0])
	if err != nil {
		t.Fatal(err)
	}
	if spec.FromVersion != "1.2.0" || spec.Version != "1.3.0" || spec.Kind != "" {
		t.Fatalf("spec = %+v", spec)
	}
	if err := s.ServiceShutdown(); err != nil {
		t.Fatal(err)
	}
}

func TestRollbackInfoJSONShape(t *testing.T) {
	raw, err := json.Marshal(RollbackInfo{})
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	var keys []string
	for k := range fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	if want := []string{"available", "reason", "reasonCode", "version"}; !reflect.DeepEqual(keys, want) {
		t.Fatalf("RollbackInfo keys = %v, want %v: the interface reads exactly these", keys, want)
	}
}

func TestRollbackOutcomeJSONCarriesItsFields(t *testing.T) {
	raw, err := json.Marshal(Outcome{Kind: outcomeRolledBack, Version: "1.2.0", From: "1.3.0", To: "1.2.0", OK: true})
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"kind", "version", "from", "to", "ok", "finishedAt"} {
		if _, ok := fields[key]; !ok {
			t.Fatalf("rollback outcome lacks %q: %s", key, raw)
		}
	}
	if fields["kind"] != "rolled_back" {
		t.Fatalf("kind = %v", fields["kind"])
	}
}

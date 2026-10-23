package selfupdate

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"typhon/internal/storage"
)

func TestPerformRollbackPutsBackTheLauncherAndOnlyTheMigratedState(t *testing.T) {
	f := newRollbackFixture(t)
	display := stubDisplayVersion(t)

	if err := performRollback(context.Background(), f.configDir, f.target, "1.3.0", "1.2.0", false); err != nil {
		t.Fatalf("performRollback: %v", err)
	}

	if got := readTestFile(t, f.target); got != f.oldExe {
		t.Fatalf("launcher = %q, want the saved %q", got, f.oldExe)
	}
	if v, ok, err := storage.EnvelopeVersion([]byte(readConfigFile(t, f.configDir, "playlog.json"))); err != nil || !ok || v != 2 {
		t.Fatalf("playlog.json envelope = %d (%v, %v), want the saved version 2", v, ok, err)
	}
	if !strings.Contains(readConfigFile(t, f.configDir, "playlog.json"), "session-a") || strings.Contains(readConfigFile(t, f.configDir, "playlog.json"), "session-b") {
		t.Fatal("playlog.json is not the saved content")
	}
	if !strings.Contains(readConfigFile(t, f.configDir, "history.json"), "entry-b") {
		t.Fatal("history.json is at its saved version and must keep the entry added after the update")
	}
	if !reflect.DeepEqual(display.set, []string{"1.2.0"}) || display.reverted != 0 {
		t.Fatalf("registry version writes = %v, reverts = %d, want one write of 1.2.0", display.set, display.reverted)
	}
	st, err := mustStore(t, f.configDir).Load()
	if err != nil {
		t.Fatal(err)
	}
	if st.SkippedVersion != "1.3.0" {
		t.Fatalf("skipped version = %q, want the version the launcher left", st.SkippedVersion)
	}
	if _, err := loadPrevious(f.configDir); !errors.Is(err, ErrNoPrevious) {
		t.Fatalf("the used snapshot is still on offer: %v", err)
	}
	dir := mustPreviousDir(t, f.configDir)
	if _, err := os.Stat(dir); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("the used snapshot directory survived: %v", err)
	}
	if left := tempNames(t, filepath.Dir(f.target)); len(left) != 0 {
		t.Fatalf("temp files beside the launcher: %v", left)
	}
}

func TestPerformRollbackPortableLeavesTheRegistryAlone(t *testing.T) {
	f := newRollbackFixture(t)
	display := stubDisplayVersion(t)

	if err := performRollback(context.Background(), f.configDir, f.target, "1.3.0", "1.2.0", true); err != nil {
		t.Fatalf("performRollback: %v", err)
	}
	if len(display.set) != 0 {
		t.Fatalf("a portable rollback wrote the uninstall entry: %v", display.set)
	}
	if got := readTestFile(t, f.target); got != f.oldExe {
		t.Fatalf("launcher = %q", got)
	}
}

// Every refusal below happens before the first write: the launcher, the state
// and the skip record are exactly what they were.
func TestPerformRollbackRefusesBeforeChangingAnything(t *testing.T) {
	cases := []struct {
		name   string
		damage func(t *testing.T, f rollbackFixture)
		from   string
		to     string
		want   error
	}{
		{name: "saved launcher has other bytes", want: ErrPreviousHash, from: "1.3.0", to: "1.2.0",
			damage: func(t *testing.T, f rollbackFixture) {
				dir := mustPreviousDir(t, f.configDir)
				writeTestFile(t, filepath.Join(dir, previousExeName), []byte("launcher 1.2.1"))
			}},
		{name: "saved launcher is missing", want: ErrPreviousBroken, from: "1.3.0", to: "1.2.0",
			damage: func(t *testing.T, f rollbackFixture) {
				dir := mustPreviousDir(t, f.configDir)
				if err := os.Remove(filepath.Join(dir, previousExeName)); err != nil {
					t.Fatal(err)
				}
			}},
		{name: "no record", want: ErrNoPrevious, from: "1.3.0", to: "1.2.0",
			damage: func(t *testing.T, f rollbackFixture) {
				p := mustPreviousRecordPath(t, f.configDir)
				if err := os.Remove(p); err != nil {
					t.Fatal(err)
				}
			}},
		{name: "damaged record", want: ErrPreviousBroken, from: "1.3.0", to: "1.2.0",
			damage: func(t *testing.T, f rollbackFixture) {
				p := mustPreviousRecordPath(t, f.configDir)
				writeTestFile(t, p, []byte("{broken"))
			}},
		{name: "a state backup was altered", want: ErrPreviousStateHash, from: "1.3.0", to: "1.2.0",
			damage: func(t *testing.T, f rollbackFixture) {
				dir := mustPreviousDir(t, f.configDir)
				writeTestFile(t, filepath.Join(dir, previousStateName, "playlog.json"), []byte(`{"version":2,"data":["altered"]}`))
			}},
		{name: "another version was requested", want: ErrPreviousVersionDiffer, from: "1.3.0", to: "1.1.0"},
		{name: "no version to leave", want: nil, from: "", to: "1.2.0"},
		{name: "no version to go to", want: nil, from: "1.3.0", to: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newRollbackFixture(t)
			display := stubDisplayVersion(t)
			if tc.damage != nil {
				tc.damage(t, f)
			}
			playlog := readConfigFile(t, f.configDir, "playlog.json")

			err := performRollback(context.Background(), f.configDir, f.target, tc.from, tc.to, false)
			if err == nil {
				t.Fatal("performRollback() error = nil")
			}
			if tc.want != nil && !errors.Is(err, tc.want) {
				t.Fatalf("error = %v, want %v", err, tc.want)
			}
			if got := readTestFile(t, f.target); got != f.newExe {
				t.Fatalf("launcher = %q: a refused rollback touched it", got)
			}
			if got := readConfigFile(t, f.configDir, "playlog.json"); got != playlog {
				t.Fatal("a refused rollback restored state")
			}
			if len(display.set) != 0 {
				t.Fatalf("a refused rollback wrote the registry: %v", display.set)
			}
			st, loadErr := mustStore(t, f.configDir).Load()
			if loadErr != nil || st.SkippedVersion != "" {
				t.Fatalf("a refused rollback skipped a version: %+v, %v", st, loadErr)
			}
		})
	}
}

func TestPerformRollbackRegistryFailureChangesNothing(t *testing.T) {
	f := newRollbackFixture(t)
	display := stubDisplayVersion(t)
	display.err = errors.New("registry is locked down")
	playlog := readConfigFile(t, f.configDir, "playlog.json")

	err := performRollback(context.Background(), f.configDir, f.target, "1.3.0", "1.2.0", false)
	if !errors.Is(err, display.err) {
		t.Fatalf("error = %v, want the registry error", err)
	}
	if got := readTestFile(t, f.target); got != f.newExe {
		t.Fatalf("launcher = %q", got)
	}
	if got := readConfigFile(t, f.configDir, "playlog.json"); got != playlog {
		t.Fatal("state was restored although the registry write failed")
	}
	if _, err := loadPrevious(f.configDir); err != nil {
		t.Fatalf("the snapshot must stay for a retry: %v", err)
	}
}

// A swap that fails after the state went back leaves the newer launcher,
// which migrates older state forward: the registry value is put back and a
// second rollback finishes the job.
func TestPerformRollbackSwapFailureRevertsTheRegistryAndCanBeRetried(t *testing.T) {
	f := newRollbackFixture(t)
	display := stubDisplayVersion(t)
	blocked := filepath.Join(t.TempDir(), "typhon.exe")
	if err := os.MkdirAll(filepath.Join(blocked, "child"), 0o700); err != nil {
		t.Fatal(err)
	}

	err := performRollback(context.Background(), f.configDir, blocked, "1.3.0", "1.2.0", false)
	if err == nil {
		t.Fatal("performRollback() error = nil with a directory in place of the launcher")
	}
	if display.reverted != 1 {
		t.Fatalf("registry reverts = %d, want 1", display.reverted)
	}
	if v := configEnvelopeVersion(t, f.configDir, "playlog.json"); v != 2 {
		t.Fatalf("playlog.json is at version %d, the state restore comes before the swap", v)
	}
	if _, err := loadPrevious(f.configDir); err != nil {
		t.Fatalf("the snapshot must stay for a retry: %v", err)
	}
	if left := tempNames(t, filepath.Dir(blocked)); len(left) != 0 {
		t.Fatalf("temp files left: %v", left)
	}

	if err := performRollback(context.Background(), f.configDir, f.target, "1.3.0", "1.2.0", false); err != nil {
		t.Fatalf("retry: %v", err)
	}
	if got := readTestFile(t, f.target); got != f.oldExe {
		t.Fatalf("launcher after the retry = %q", got)
	}
}

// The worker can die after the state is back and before the swap. The next
// attempt finds nothing left to restore and completes the swap.
func TestPerformRollbackResumesAfterTheWorkerDiedBetweenSteps(t *testing.T) {
	f := newRollbackFixture(t)
	display := stubDisplayVersion(t)
	rec, err := loadPrevious(f.configDir)
	if err != nil {
		t.Fatal(err)
	}
	items, err := planStateRestore(context.Background(), f.configDir, rec)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("restoring %d files, want 1", len(items))
	}
	if err := applyStateRestore(context.Background(), items); err != nil {
		t.Fatal(err)
	}
	if got := readTestFile(t, f.target); got != f.newExe {
		t.Fatalf("setup: launcher = %q", got)
	}

	again, err := planStateRestore(context.Background(), f.configDir, rec)
	if err != nil {
		t.Fatal(err)
	}
	if len(again) != 0 {
		t.Fatalf("a second pass would restore %d files again", len(again))
	}
	if err := performRollback(context.Background(), f.configDir, f.target, "1.3.0", "1.2.0", false); err != nil {
		t.Fatalf("performRollback after the interruption: %v", err)
	}
	if got := readTestFile(t, f.target); got != f.oldExe {
		t.Fatalf("launcher = %q", got)
	}
	if len(display.set) != 1 {
		t.Fatalf("registry writes = %v", display.set)
	}
}

func TestPerformRollbackCancelledContextChangesNothing(t *testing.T) {
	f := newRollbackFixture(t)
	stubDisplayVersion(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := performRollback(ctx, f.configDir, f.target, "1.3.0", "1.2.0", false); !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
	if got := readTestFile(t, f.target); got != f.newExe {
		t.Fatalf("launcher = %q", got)
	}
}

func TestPerformRollbackUpdaterStateFromTheFutureStopsBeforeAnyWrite(t *testing.T) {
	f := newRollbackFixture(t)
	display := stubDisplayVersion(t)
	// A state.json this build cannot load makes the skip record impossible:
	// the rollback has to stop there, not go on without recording it.
	statePath := filepath.Join(f.configDir, "selfupdate", "state.json")
	if err := storage.Save(statePath, StateVersion+1, stored{}); err != nil {
		t.Fatal(err)
	}

	err := performRollback(context.Background(), f.configDir, f.target, "1.3.0", "1.2.0", false)
	if err == nil {
		t.Fatal("performRollback() error = nil")
	}
	if got := readTestFile(t, f.target); got != f.newExe {
		t.Fatalf("launcher = %q", got)
	}
	if len(display.set) != 0 {
		t.Fatalf("registry written: %v", display.set)
	}
	if v := configEnvelopeVersion(t, f.configDir, "playlog.json"); v != 3 {
		t.Fatalf("playlog.json was restored (version %d) although the rollback stopped", v)
	}
}

func TestCheckRollback(t *testing.T) {
	cases := []struct {
		name      string
		supported error
		current   string
		damage    func(t *testing.T, f rollbackFixture)
		available bool
		wantCode  string
	}{
		{name: "available", current: "1.3.0", available: true},
		{name: "available from a later build", current: "1.3.4", available: true},
		{name: "unsupported platform", supported: ErrRollbackUnsupported, current: "1.3.0", wantCode: "selfupdate.rollback_unsupported"},
		{name: "running the saved version", current: "1.2.0", wantCode: "selfupdate.rollback_not_older"},
		{name: "running an even older version", current: "1.1.0", wantCode: "selfupdate.rollback_not_older"},
		{name: "running version is not comparable", current: "dev-build", wantCode: "selfupdate.invalid_version"},
		{name: "nothing saved", current: "1.3.0", wantCode: "selfupdate.rollback_none",
			damage: func(t *testing.T, f rollbackFixture) {
				if err := dropPrevious(f.configDir); err != nil {
					t.Fatal(err)
				}
			}},
		{name: "saved launcher missing", current: "1.3.0", wantCode: "selfupdate.rollback_broken",
			damage: func(t *testing.T, f rollbackFixture) {
				dir := mustPreviousDir(t, f.configDir)
				if err := os.Remove(filepath.Join(dir, previousExeName)); err != nil {
					t.Fatal(err)
				}
			}},
		{name: "saved launcher has the wrong size", current: "1.3.0", wantCode: "selfupdate.rollback_broken",
			damage: func(t *testing.T, f rollbackFixture) {
				dir := mustPreviousDir(t, f.configDir)
				writeTestFile(t, filepath.Join(dir, previousExeName), []byte("short"))
			}},
		{name: "damaged record", current: "1.3.0", wantCode: "selfupdate.rollback_broken",
			damage: func(t *testing.T, f rollbackFixture) {
				p := mustPreviousRecordPath(t, f.configDir)
				writeTestFile(t, p, []byte("nope"))
			}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newRollbackFixture(t)
			stubRollbackSupported(t, tc.supported)
			if tc.damage != nil {
				tc.damage(t, f)
			}

			info := rollbackAvailability(f.configDir, tc.current)

			if info.Available != tc.available {
				t.Fatalf("info = %+v, want available = %v", info, tc.available)
			}
			if info.Available {
				if info.Version != "1.2.0" || info.Reason != "" || info.ReasonCode != "" {
					t.Fatalf("info = %+v", info)
				}
				return
			}
			if info.Reason == "" {
				t.Fatalf("info = %+v: an unavailable rollback says why", info)
			}
			if info.ReasonCode != tc.wantCode {
				t.Fatalf("ReasonCode = %q, want %q", info.ReasonCode, tc.wantCode)
			}
			if info.Version != "" {
				t.Fatalf("an unavailable rollback names a version: %+v", info)
			}
		})
	}
}

func TestSameVersion(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"1.2.0", "1.2.0", true},
		{"1.2.0", "v1.2.0", true},
		{"1.2.0", "1.2.1", false},
		{"1.10.0", "1.9.0", false},
		{"", "1.2.0", false},
		{"dev", "dev", true},
		{"dev", "1.2.0", false},
	}
	for _, tc := range cases {
		if got := sameVersion(tc.a, tc.b); got != tc.want {
			t.Errorf("sameVersion(%q, %q) = %v, want %v", tc.a, tc.b, got, tc.want)
		}
	}
}

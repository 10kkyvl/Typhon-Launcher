package selfupdate

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

type rollBackCall struct {
	configDir, target, from, to string
	portable                    bool
}

func stubRollBack(t *testing.T, result error) *[]rollBackCall {
	t.Helper()
	var calls []rollBackCall
	restore := rollBack
	rollBack = func(_ context.Context, configDir, target, from, to string, portable bool) error {
		calls = append(calls, rollBackCall{configDir, target, from, to, portable})
		return result
	}
	t.Cleanup(func() { rollBack = restore })
	return &calls
}

func rollbackSpec(target string) updateSpec {
	return updateSpec{
		Kind:         specKindRollback,
		Language:     "en",
		InstallDir:   filepath.Dir(target),
		ParentPID:    4242,
		RelaunchPath: target,
		Version:      "1.2.0",
		FromVersion:  "1.3.0",
	}
}

func TestRunWorkerRollbackRecordsTheOutcomeAndRelaunches(t *testing.T) {
	shortWorkerTimeouts(t)
	configDir := testConfigDir(t)
	target := filepath.Join(t.TempDir(), "typhon.exe")
	writeTestFile(t, target, []byte("launcher"))
	fakes := &fakePrimitives{aliveUntil: 1}
	installFakePrimitives(t, fakes)
	calls := stubRollBack(t, nil)

	spec := rollbackSpec(target)
	spec.Portable = true
	specPath := writeWorkerSpec(t, configDir, spec)
	ui := &recordingReporter{}
	err := runWorker(specPath, func(title, detail string) stageReporter {
		if title != "Rolling Typhon back to 1.2.0" {
			t.Fatalf("worker title = %q", title)
		}
		return ui
	})
	if err != nil {
		t.Fatalf("runWorker: %v", err)
	}

	want := []rollBackCall{{configDir, target, "1.3.0", "1.2.0", true}}
	if !reflect.DeepEqual(*calls, want) {
		t.Fatalf("rollback calls = %+v, want %+v", *calls, want)
	}
	if fakes.applyCalls != 0 {
		t.Fatalf("a rollback ran the installer %d times", fakes.applyCalls)
	}
	if !reflect.DeepEqual(fakes.relaunchCalls, []string{target}) {
		t.Fatalf("relaunch calls = %v", fakes.relaunchCalls)
	}
	got := readWorkerOutcome(t, configDir)
	if !got.OK || got.Kind != outcomeRolledBack || got.From != "1.3.0" || got.To != "1.2.0" || got.Version != "1.2.0" || got.Error != "" {
		t.Fatalf("outcome = %+v", got)
	}
	if len(ui.failed) != 0 || len(ui.stages) != 2 || !strings.Contains(ui.stages[1], "Starting Typhon") {
		t.Fatalf("stages = %v, failed = %v", ui.stages, ui.failed)
	}
}

func TestRunWorkerRollbackFailureIsRecordedAndTheLauncherStillComesBack(t *testing.T) {
	shortWorkerTimeouts(t)
	configDir := testConfigDir(t)
	target := filepath.Join(t.TempDir(), "typhon.exe")
	writeTestFile(t, target, []byte("launcher"))
	fakes := &fakePrimitives{}
	installFakePrimitives(t, fakes)
	stubRollBack(t, ErrPreviousHash)

	specPath := writeWorkerSpec(t, configDir, rollbackSpec(target))
	ui := &recordingReporter{}
	err := runWorker(specPath, func(string, string) stageReporter { return ui })
	if !errors.Is(err, ErrPreviousHash) {
		t.Fatalf("runWorker error = %v, want ErrPreviousHash", err)
	}

	got := readWorkerOutcome(t, configDir)
	if got.OK || got.Kind != outcomeRolledBack || got.From != "1.3.0" || got.To != "1.2.0" || !strings.Contains(got.Error, "selfupdate.rollback_hash") {
		t.Fatalf("outcome = %+v, want a failed rollback carrying the error code", got)
	}
	if !reflect.DeepEqual(fakes.relaunchCalls, []string{target}) {
		t.Fatalf("relaunch calls = %v: the launcher that is still installed has to come back", fakes.relaunchCalls)
	}
	if len(ui.stages) < 1 || !strings.Contains(ui.stages[len(ui.stages)-1], "Could not roll Typhon back") {
		t.Fatalf("stages = %v", ui.stages)
	}
}

func TestRunWorkerRollbackWaitsForTheParentAndGivesUp(t *testing.T) {
	shortWorkerTimeouts(t)
	configDir := testConfigDir(t)
	target := filepath.Join(t.TempDir(), "typhon.exe")
	writeTestFile(t, target, []byte("launcher"))
	fakes := &fakePrimitives{aliveUntil: 1 << 30}
	installFakePrimitives(t, fakes)
	calls := stubRollBack(t, nil)

	specPath := writeWorkerSpec(t, configDir, rollbackSpec(target))
	ui := &recordingReporter{}
	err := runWorker(specPath, func(string, string) stageReporter { return ui })
	if !errors.Is(err, errParentStillRunning) {
		t.Fatalf("runWorker error = %v, want errParentStillRunning", err)
	}
	if len(*calls) != 0 {
		t.Fatalf("rollback ran while the launcher was still open: %+v", *calls)
	}
	if len(fakes.relaunchCalls) != 0 {
		t.Fatalf("relaunch calls = %v", fakes.relaunchCalls)
	}
	if got := readWorkerOutcome(t, configDir); got.OK || got.Kind != outcomeRolledBack {
		t.Fatalf("outcome = %+v", got)
	}
}

func TestRunWorkerRollbackEndToEnd(t *testing.T) {
	shortWorkerTimeouts(t)
	configDir := testConfigDir(t)
	f := newRollbackFixtureAt(t, configDir)
	display := stubDisplayVersion(t)
	fakes := &fakePrimitives{}
	installFakePrimitives(t, fakes)

	specPath := writeWorkerSpec(t, configDir, rollbackSpec(f.target))
	if err := runWorker(specPath, quietReporter); err != nil {
		t.Fatalf("runWorker: %v", err)
	}

	if got := readTestFile(t, f.target); got != f.oldExe {
		t.Fatalf("launcher = %q, want %q", got, f.oldExe)
	}
	if !reflect.DeepEqual(display.set, []string{"1.2.0"}) {
		t.Fatalf("registry writes = %v", display.set)
	}
	if got := readWorkerOutcome(t, configDir); !got.OK || got.Kind != outcomeRolledBack {
		t.Fatalf("outcome = %+v", got)
	}
	if !reflect.DeepEqual(fakes.relaunchCalls, []string{f.target}) {
		t.Fatalf("relaunch calls = %v", fakes.relaunchCalls)
	}
}

func TestRunWorkerUpdateSavesTheRunningLauncherBeforeTheInstaller(t *testing.T) {
	shortWorkerTimeouts(t)
	stubRollbackSupported(t, nil)
	configDir := testConfigDir(t)
	target := filepath.Join(t.TempDir(), "typhon.exe")
	writeTestFile(t, target, []byte("old launcher"))
	fakes := &fakePrimitives{applyWrites: []byte("new launcher")}
	installFakePrimitives(t, fakes)

	var order []string
	var saved []string
	saveCurrent = func(_ context.Context, dir, exe, ver string) error {
		order = append(order, "save")
		saved = []string{dir, exe, ver}
		if got := readTestFile(t, target); got != "old launcher" {
			t.Errorf("the launcher was already replaced when it was saved: %q", got)
		}
		return nil
	}
	applyInstaller = func(_ context.Context, _, _, target string) error {
		order = append(order, "install")
		writeTestFile(t, target, []byte("new launcher"))
		return nil
	}

	spec := updateSpec{InstallerPath: "setup", InstallDir: filepath.Dir(target), ParentPID: 4242, RelaunchPath: target, Version: "1.3.0", FromVersion: "1.2.0"}
	specPath := writeWorkerSpec(t, configDir, spec)
	if err := runWorker(specPath, quietReporter); err != nil {
		t.Fatalf("runWorker: %v", err)
	}
	if !reflect.DeepEqual(order, []string{"save", "install"}) {
		t.Fatalf("order = %v", order)
	}
	if want := []string{configDir, target, "1.2.0"}; !reflect.DeepEqual(saved, want) {
		t.Fatalf("saveCurrent args = %v, want %v", saved, want)
	}
	if got := readWorkerOutcome(t, configDir); !got.OK || got.Kind != "" {
		t.Fatalf("outcome = %+v: an update leaves Kind empty", got)
	}
}

func TestRunWorkerUpdateStopsWhenTheLauncherCannotBeSaved(t *testing.T) {
	shortWorkerTimeouts(t)
	stubRollbackSupported(t, nil)
	configDir := testConfigDir(t)
	target := filepath.Join(t.TempDir(), "typhon.exe")
	writeTestFile(t, target, []byte("old launcher"))
	fakes := &fakePrimitives{applyWrites: []byte("new launcher")}
	installFakePrimitives(t, fakes)
	errDisk := errors.New("disk is full")
	saveCurrent = func(context.Context, string, string, string) error { return errDisk }

	spec := updateSpec{InstallerPath: "setup", InstallDir: filepath.Dir(target), ParentPID: 4242, RelaunchPath: target, Version: "1.3.0", FromVersion: "1.2.0"}
	err := runWorker(writeWorkerSpec(t, configDir, spec), quietReporter)
	if !errors.Is(err, errDisk) {
		t.Fatalf("runWorker error = %v, want the save error", err)
	}
	if fakes.applyCalls != 0 {
		t.Fatalf("the installer ran %d times without a saved launcher", fakes.applyCalls)
	}
	if got := readTestFile(t, target); got != "old launcher" {
		t.Fatalf("launcher = %q", got)
	}
	if got := readWorkerOutcome(t, configDir); got.OK || !strings.Contains(got.Error, "disk is full") {
		t.Fatalf("outcome = %+v", got)
	}
	if !reflect.DeepEqual(fakes.relaunchCalls, []string{target}) {
		t.Fatalf("relaunch calls = %v: the old launcher has to come back", fakes.relaunchCalls)
	}
}

func TestRunWorkerUpdateOnAPlatformWithoutRollbackDoesNotSave(t *testing.T) {
	shortWorkerTimeouts(t)
	configDir := testConfigDir(t)
	target := filepath.Join(t.TempDir(), "typhon.exe")
	writeTestFile(t, target, []byte("old launcher"))
	fakes := &fakePrimitives{applyWrites: []byte("new launcher")}
	installFakePrimitives(t, fakes)
	stubRollbackSupported(t, ErrRollbackUnsupported)
	saveCurrent = func(context.Context, string, string, string) error {
		t.Error("saveCurrent called on a platform that cannot roll back")
		return nil
	}

	spec := updateSpec{InstallerPath: "setup", InstallDir: filepath.Dir(target), ParentPID: 4242, RelaunchPath: target, Version: "1.3.0"}
	if err := runWorker(writeWorkerSpec(t, configDir, spec), quietReporter); err != nil {
		t.Fatalf("runWorker: %v", err)
	}
	if fakes.applyCalls != 1 {
		t.Fatalf("installer ran %d times, want 1", fakes.applyCalls)
	}
}

func TestReadUpdateSpecKinds(t *testing.T) {
	cases := []struct {
		kind    string
		wantErr bool
	}{
		{"", false},
		{specKindUpdate, false},
		{specKindRollback, false},
		{"downgrade", true},
		{"ROLLBACK", true},
	}
	for _, tc := range cases {
		t.Run("kind "+tc.kind, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "spec.json")
			if err := writeUpdateSpec(path, updateSpec{Kind: tc.kind, Version: "1.2.0"}); err != nil {
				t.Fatal(err)
			}
			spec, err := readUpdateSpec(path)
			if (err != nil) != tc.wantErr {
				t.Fatalf("error = %v, want error: %v", err, tc.wantErr)
			}
			if err == nil && spec.Kind != tc.kind {
				t.Fatalf("kind = %q", spec.Kind)
			}
		})
	}
}

func TestOutcomeOfAnUpdateCarriesNoRollbackFields(t *testing.T) {
	configDir := testConfigDir(t)
	path, err := OutcomePath(configDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeOutcome(path, Outcome{Version: "1.3.0", OK: true}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{`"kind"`, `"from"`, `"to"`} {
		if strings.Contains(string(raw), field) {
			t.Fatalf("an update outcome carries %s: %s", field, raw)
		}
	}
}

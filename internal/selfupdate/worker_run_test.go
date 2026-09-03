package selfupdate

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// testConfigDir points settings.ConfigDir at a fresh temp dir on every OS
// and returns the launcher's config dir inside it.
func testConfigDir(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	switch runtime.GOOS {
	case "windows":
		t.Setenv("APPDATA", home)
	case "darwin":
		t.Setenv("HOME", home)
	default:
		t.Setenv("XDG_CONFIG_HOME", home)
	}
	base, err := os.UserConfigDir()
	if err != nil {
		t.Fatalf("UserConfigDir: %v", err)
	}
	return filepath.Join(base, "Typhon")
}

func quietReporter(string, string) stageReporter { return silentReporter{} }

type recordingReporter struct {
	stages []string
	failed []string
}

func (r *recordingReporter) setStage(title, detail string) {
	r.stages = append(r.stages, title+"|"+detail)
}
func (r *recordingReporter) fail(title, detail string) { r.failed = append(r.failed, title+"|"+detail) }
func (r *recordingReporter) wait()                     {}
func (r *recordingReporter) close()                    {}

func shortWorkerTimeouts(t *testing.T) {
	t.Helper()
	restoreWait, restorePoll := parentExitTimeout, parentPollInterval
	parentExitTimeout = 100 * time.Millisecond
	parentPollInterval = 5 * time.Millisecond
	t.Cleanup(func() { parentExitTimeout, parentPollInterval = restoreWait, restorePoll })
}

type fakePrimitives struct {
	aliveCalls    int
	aliveUntil    int
	applyCalls    int
	applyErr      error
	applyWrites   []byte
	relaunchCalls []string
	relaunchErr   error
}

func installFakePrimitives(t *testing.T, f *fakePrimitives) {
	t.Helper()
	restoreAlive, restoreApply, restoreRelaunch := parentAlive, applyInstaller, relaunchLauncher
	parentAlive = func(int) (bool, error) {
		f.aliveCalls++
		return f.aliveCalls <= f.aliveUntil, nil
	}
	applyInstaller = func(_ context.Context, _, _, target string) error {
		f.applyCalls++
		if f.applyErr != nil {
			return f.applyErr
		}
		if f.applyWrites != nil {
			writeTestFile(t, target, f.applyWrites)
		}
		return nil
	}
	relaunchLauncher = func(path string) error {
		f.relaunchCalls = append(f.relaunchCalls, path)
		return f.relaunchErr
	}
	t.Cleanup(func() { parentAlive, applyInstaller, relaunchLauncher = restoreAlive, restoreApply, restoreRelaunch })
}

func writeWorkerSpec(t *testing.T, configDir string, spec updateSpec) string {
	t.Helper()
	specPath, err := SpecPath(configDir)
	if err != nil {
		t.Fatalf("SpecPath: %v", err)
	}
	if err := writeUpdateSpec(specPath, spec); err != nil {
		t.Fatalf("writeUpdateSpec: %v", err)
	}
	return specPath
}

func readWorkerOutcome(t *testing.T, configDir string) Outcome {
	t.Helper()
	outcomePath, err := OutcomePath(configDir)
	if err != nil {
		t.Fatalf("OutcomePath: %v", err)
	}
	got, err := readOutcome(outcomePath)
	if err != nil {
		t.Fatalf("readOutcome: %v", err)
	}
	return got
}

func TestRunWorkerAppliesAndRelaunchesOnceParentExits(t *testing.T) {
	shortWorkerTimeouts(t)
	configDir := testConfigDir(t)
	installDir := t.TempDir()
	target := filepath.Join(installDir, "typhon")
	writeTestFile(t, target, []byte("old launcher"))

	fakes := &fakePrimitives{aliveUntil: 2, applyWrites: []byte("new launcher")}
	installFakePrimitives(t, fakes)

	specPath := writeWorkerSpec(t, configDir, updateSpec{
		InstallerPath: filepath.Join(installDir, "setup"),
		InstallDir:    installDir,
		ParentPID:     4242,
		RelaunchPath:  target,
		Version:       "2.0.0",
	})

	ui := &recordingReporter{}
	if err := runWorker(specPath, func(string, string) stageReporter { return ui }); err != nil {
		t.Fatalf("runWorker: %v", err)
	}

	if fakes.aliveCalls != 3 {
		t.Fatalf("parentAlive calls = %d, want 3 (two alive polls, then gone)", fakes.aliveCalls)
	}
	if fakes.applyCalls != 1 {
		t.Fatalf("apply calls = %d, want 1", fakes.applyCalls)
	}
	if len(fakes.relaunchCalls) != 1 || fakes.relaunchCalls[0] != target {
		t.Fatalf("relaunch calls = %v, want [%s]", fakes.relaunchCalls, target)
	}
	got := readWorkerOutcome(t, configDir)
	if !got.OK || got.Version != "2.0.0" || got.Error != "" {
		t.Fatalf("outcome = %+v, want ok for 2.0.0", got)
	}
	if _, err := os.Stat(specPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("spec file survived runWorker: %v", err)
	}
	if len(ui.failed) != 0 {
		t.Fatalf("fail() called: %v", ui.failed)
	}
	if len(ui.stages) != 2 || !strings.Contains(ui.stages[1], "запускаем") {
		t.Fatalf("stages = %v, want install then relaunch", ui.stages)
	}
}

func TestRunWorkerGivesUpWhenParentNeverExits(t *testing.T) {
	shortWorkerTimeouts(t)
	configDir := testConfigDir(t)
	installDir := t.TempDir()
	target := filepath.Join(installDir, "typhon")
	writeTestFile(t, target, []byte("old launcher"))

	fakes := &fakePrimitives{aliveUntil: 1 << 30, applyWrites: []byte("new launcher")}
	installFakePrimitives(t, fakes)

	specPath := writeWorkerSpec(t, configDir, updateSpec{
		InstallerPath: filepath.Join(installDir, "setup"),
		InstallDir:    installDir,
		ParentPID:     4242,
		RelaunchPath:  target,
		Version:       "2.0.0",
	})

	ui := &recordingReporter{}
	err := runWorker(specPath, func(string, string) stageReporter { return ui })
	if !errors.Is(err, errParentStillRunning) {
		t.Fatalf("runWorker error = %v, want errParentStillRunning", err)
	}
	if fakes.applyCalls != 0 {
		t.Fatalf("apply calls = %d, want 0", fakes.applyCalls)
	}
	if len(fakes.relaunchCalls) != 0 {
		t.Fatalf("relaunch calls = %v, want none: the launcher is still on screen", fakes.relaunchCalls)
	}
	got := readWorkerOutcome(t, configDir)
	if got.OK || !strings.Contains(got.Error, errParentStillRunning.Error()) {
		t.Fatalf("outcome = %+v, want a failure naming the parent timeout", got)
	}
	if len(ui.failed) != 1 {
		t.Fatalf("fail() calls = %v, want exactly one", ui.failed)
	}
	if data, err := os.ReadFile(target); err != nil || string(data) != "old launcher" {
		t.Fatalf("target = %q, %v; want untouched", data, err)
	}
}

func TestRunWorkerRecordsApplyErrorAndStillRelaunches(t *testing.T) {
	shortWorkerTimeouts(t)
	configDir := testConfigDir(t)
	installDir := t.TempDir()
	target := filepath.Join(installDir, "typhon")
	writeTestFile(t, target, []byte("old launcher"))

	errBoom := errors.New("installer exploded")
	fakes := &fakePrimitives{aliveUntil: 0, applyErr: errBoom}
	installFakePrimitives(t, fakes)

	specPath := writeWorkerSpec(t, configDir, updateSpec{
		InstallerPath: filepath.Join(installDir, "setup"),
		InstallDir:    installDir,
		ParentPID:     4242,
		RelaunchPath:  target,
		Version:       "2.0.0",
	})

	err := runWorker(specPath, quietReporter)
	if !errors.Is(err, errBoom) {
		t.Fatalf("runWorker error = %v, want errBoom", err)
	}
	if len(fakes.relaunchCalls) != 1 {
		t.Fatalf("relaunch calls = %v, want one: the old launcher must come back", fakes.relaunchCalls)
	}
	got := readWorkerOutcome(t, configDir)
	if got.OK || !strings.Contains(got.Error, errBoom.Error()) {
		t.Fatalf("outcome = %+v, want the apply failure recorded", got)
	}
}

func TestRunWorkerReportsUntouchedLauncher(t *testing.T) {
	shortWorkerTimeouts(t)
	configDir := testConfigDir(t)
	installDir := t.TempDir()
	target := filepath.Join(installDir, "typhon")
	writeTestFile(t, target, []byte("old launcher"))

	fakes := &fakePrimitives{aliveUntil: 0}
	installFakePrimitives(t, fakes)

	specPath := writeWorkerSpec(t, configDir, updateSpec{
		InstallerPath: filepath.Join(installDir, "setup"),
		InstallDir:    installDir,
		ParentPID:     4242,
		RelaunchPath:  target,
		Version:       "2.0.0",
	})

	err := runWorker(specPath, quietReporter)
	if !errors.Is(err, ErrNotReplaced) {
		t.Fatalf("runWorker error = %v, want ErrNotReplaced when apply leaves the binary as it was", err)
	}
	if len(fakes.relaunchCalls) != 1 {
		t.Fatalf("relaunch calls = %v, want one", fakes.relaunchCalls)
	}
	if got := readWorkerOutcome(t, configDir); got.OK {
		t.Fatalf("outcome = %+v, want a failure", got)
	}
}

func TestRunWorkerSurfacesRelaunchFailure(t *testing.T) {
	shortWorkerTimeouts(t)
	configDir := testConfigDir(t)
	installDir := t.TempDir()
	target := filepath.Join(installDir, "typhon")

	errLaunch := errors.New("exec format error")
	fakes := &fakePrimitives{aliveUntil: 0, applyWrites: []byte("new launcher"), relaunchErr: errLaunch}
	installFakePrimitives(t, fakes)

	specPath := writeWorkerSpec(t, configDir, updateSpec{
		InstallerPath: filepath.Join(installDir, "setup"),
		InstallDir:    installDir,
		ParentPID:     4242,
		RelaunchPath:  target,
		Version:       "2.0.0",
	})

	ui := &recordingReporter{}
	err := runWorker(specPath, func(string, string) stageReporter { return ui })
	if !errors.Is(err, errLaunch) {
		t.Fatalf("runWorker error = %v, want errLaunch", err)
	}
	if got := readWorkerOutcome(t, configDir); !got.OK {
		t.Fatalf("outcome = %+v, want ok: the install itself succeeded", got)
	}
	if len(ui.failed) != 1 {
		t.Fatalf("fail() calls = %v, want the relaunch failure shown", ui.failed)
	}
}

func TestRunWorkerMissingSpec(t *testing.T) {
	installFakePrimitives(t, &fakePrimitives{})
	if err := runWorker(filepath.Join(t.TempDir(), "missing.json"), quietReporter); err == nil {
		t.Fatal("runWorker on a missing spec returned nil error")
	}
}

func TestRunWorkerCorruptSpecIsRemoved(t *testing.T) {
	installFakePrimitives(t, &fakePrimitives{})
	specPath := filepath.Join(t.TempDir(), "spec.json")
	writeTestFile(t, specPath, []byte("{not json"))
	if err := runWorker(specPath, quietReporter); err == nil {
		t.Fatal("runWorker on a corrupt spec returned nil error")
	}
	if _, err := os.Stat(specPath); err != nil {
		t.Fatalf("spec stat = %v; a spec that never parsed is not the worker's to delete", err)
	}
}

func TestWaitForParentExitHonoursContext(t *testing.T) {
	shortWorkerTimeouts(t)
	installFakePrimitives(t, &fakePrimitives{aliveUntil: 1 << 30})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := waitForParentExit(ctx, 4242); !errors.Is(err, errParentStillRunning) {
		t.Fatalf("waitForParentExit = %v, want errParentStillRunning", err)
	}
}

func TestWaitForParentExitPropagatesProbeError(t *testing.T) {
	shortWorkerTimeouts(t)
	errProbe := errors.New("probe failed")
	restore := parentAlive
	parentAlive = func(int) (bool, error) { return false, errProbe }
	t.Cleanup(func() { parentAlive = restore })
	if err := waitForParentExit(context.Background(), 4242); !errors.Is(err, errProbe) {
		t.Fatalf("waitForParentExit = %v, want errProbe", err)
	}
}

func TestEnsureReplaced(t *testing.T) {
	tests := []struct {
		name          string
		before, after string
		want          error
	}{
		{name: "missing after", before: "a", after: "", want: ErrNotReplaced},
		{name: "identical", before: "a", after: "a", want: ErrNotReplaced},
		{name: "changed", before: "a", after: "b"},
		{name: "created", before: "", after: "b"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ensureReplaced(tt.before, tt.after, "typhon")
			if tt.want == nil && err != nil {
				t.Fatalf("ensureReplaced = %v, want nil", err)
			}
			if tt.want != nil && !errors.Is(err, tt.want) {
				t.Fatalf("ensureReplaced = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestNewWorkerContextsAreIndependent(t *testing.T) {
	restoreWait, restoreApply := parentExitTimeout, applyTimeout
	parentExitTimeout = 20 * time.Millisecond
	applyTimeout = 5 * time.Second
	t.Cleanup(func() { parentExitTimeout, applyTimeout = restoreWait, restoreApply })

	waitCtx, applyCtx, cancel := newWorkerContexts(context.Background())
	defer cancel()

	<-waitCtx.Done()
	if applyCtx.Err() != nil {
		t.Fatalf("applyCtx.Err() = %v, want nil: the installer budget must not be spent waiting for the parent", applyCtx.Err())
	}
}

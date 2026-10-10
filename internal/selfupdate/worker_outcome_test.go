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
	"time"
)

func TestRunWorkerRelaunchesWhenTheOutcomeCannotBeRecorded(t *testing.T) {
	shortWorkerTimeouts(t)
	configDir := testConfigDir(t)
	installDir := t.TempDir()
	target := filepath.Join(installDir, "typhon")
	writeTestFile(t, target, []byte("old launcher"))
	writeTestFile(t, filepath.Join(configDir, "selfupdate"), []byte("a file where the cache directory belongs"))

	fakes := &fakePrimitives{aliveUntil: 0, applyWrites: []byte("new launcher")}
	installFakePrimitives(t, fakes)

	specPath := filepath.Join(t.TempDir(), "update-spec.json")
	if err := writeUpdateSpec(specPath, updateSpec{
		InstallerPath: filepath.Join(installDir, "setup"),
		InstallDir:    installDir,
		ParentPID:     4242,
		RelaunchPath:  target,
		Version:       "2.0.0",
	}); err != nil {
		t.Fatalf("writeUpdateSpec: %v", err)
	}

	err := runWorker(specPath, quietReporter)
	if err == nil || !strings.Contains(err.Error(), "outcome") {
		t.Fatalf("runWorker error = %v, want the unrecorded outcome reported", err)
	}
	if len(fakes.relaunchCalls) != 1 || fakes.relaunchCalls[0] != target {
		t.Fatalf("relaunch calls = %v, want the launcher back even though its result could not be saved", fakes.relaunchCalls)
	}
	if fakes.applyCalls != 1 {
		t.Fatalf("apply calls = %d, want 1", fakes.applyCalls)
	}
	if _, err := os.Stat(specPath); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("spec survived the run: %v", err)
	}
}

func TestRunWorkerStopsAnInstallerThatOverrunsItsBudget(t *testing.T) {
	shortWorkerTimeouts(t)
	prev := applyTimeout
	applyTimeout = 100 * time.Millisecond
	t.Cleanup(func() { applyTimeout = prev })

	configDir := testConfigDir(t)
	installDir := t.TempDir()
	target := filepath.Join(installDir, "typhon")
	writeTestFile(t, target, []byte("old launcher"))

	fakes := &fakePrimitives{aliveUntil: 0}
	installFakePrimitives(t, fakes)
	applyInstaller = func(ctx context.Context, _, _, _ string) error {
		<-ctx.Done()
		return ctx.Err()
	}

	specPath := writeWorkerSpec(t, configDir, updateSpec{
		InstallerPath: filepath.Join(installDir, "setup"),
		InstallDir:    installDir,
		ParentPID:     4242,
		RelaunchPath:  target,
		Version:       "2.0.0",
	})

	err := runWorker(specPath, quietReporter)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("runWorker error = %v, want context.DeadlineExceeded", err)
	}
	got := readWorkerOutcome(t, configDir)
	if got.OK || got.Version != "2.0.0" || !strings.Contains(got.Error, context.DeadlineExceeded.Error()) {
		t.Fatalf("outcome = %+v, want a failure that names the timeout", got)
	}
	if len(fakes.relaunchCalls) != 1 || fakes.relaunchCalls[0] != target {
		t.Fatalf("relaunch calls = %v, want the previous launcher started again", fakes.relaunchCalls)
	}
	if data, err := os.ReadFile(target); err != nil || string(data) != "old launcher" {
		t.Fatalf("launcher = %q, %v; want it untouched", data, err)
	}
}

func TestRunWorkerReportsBothAFailedInstallAndAFailedRelaunch(t *testing.T) {
	shortWorkerTimeouts(t)
	configDir := testConfigDir(t)
	installDir := t.TempDir()
	target := filepath.Join(installDir, "typhon")
	writeTestFile(t, target, []byte("old launcher"))

	errInstall := errors.New("installer exploded")
	errLaunch := errors.New("launcher binary is gone")
	fakes := &fakePrimitives{aliveUntil: 0, applyErr: errInstall, relaunchErr: errLaunch}
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
	if !errors.Is(err, errInstall) || !errors.Is(err, errLaunch) {
		t.Fatalf("runWorker error = %v, want both the install and the relaunch failure", err)
	}
	if got := readWorkerOutcome(t, configDir); got.OK || !strings.Contains(got.Error, errInstall.Error()) {
		t.Fatalf("outcome = %+v, want the install failure recorded before the relaunch was tried", got)
	}
	if len(ui.failed) != 1 {
		t.Fatalf("fail() calls = %v, want the user told the launcher did not restart", ui.failed)
	}
}

func TestOutcomeFileRoundTripAndUnreadableRecords(t *testing.T) {
	t.Run("round trip", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "selfupdate", "last-update.json")
		want := Outcome{Version: "1.2.3", OK: false, Error: "installer exited with code 5", FinishedAt: time.Date(2026, 9, 2, 12, 30, 0, 0, time.UTC)}
		if err := writeOutcome(path, want); err != nil {
			t.Fatalf("writeOutcome: %v", err)
		}
		got, err := readOutcome(path)
		if err != nil {
			t.Fatalf("readOutcome: %v", err)
		}
		if got.Version != want.Version || got.OK != want.OK || got.Error != want.Error || !got.FinishedAt.Equal(want.FinishedAt) {
			t.Fatalf("readOutcome() = %+v, want %+v", got, want)
		}
	})

	t.Run("missing record is reported as missing", func(t *testing.T) {
		_, err := readOutcome(filepath.Join(t.TempDir(), "last-update.json"))
		if !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("readOutcome() error = %v, want fs.ErrNotExist", err)
		}
	})

	for name, raw := range map[string]string{
		"empty":      "",
		"garbage":    "not json",
		"truncated":  `{"version":1,"data":{"version":"1.2.3","ok":`,
		"newer":      `{"version":9,"data":{"ok":true}}`,
		"wrong type": `{"version":1,"data":{"ok":"yes"}}`,
	} {
		t.Run(name+" record is an error, not a missing one", func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "last-update.json")
			writeTestFile(t, path, []byte(raw))
			got, err := readOutcome(path)
			if err == nil || errors.Is(err, fs.ErrNotExist) {
				t.Fatalf("readOutcome() = %+v, %v; want an error that is not fs.ErrNotExist", got, err)
			}
			if got != (Outcome{}) {
				t.Fatalf("readOutcome() = %+v next to an error", got)
			}
		})
	}
}

func TestWorkerTextCoversEveryLabelInBothLanguages(t *testing.T) {
	for _, lang := range []string{"ru", "en", "", "de"} {
		t.Run("language "+lang, func(t *testing.T) {
			labels := reflect.ValueOf(workerText(lang))
			for i := 0; i < labels.NumField(); i++ {
				if labels.Field(i).String() == "" {
					t.Errorf("label %s is empty: the progress window would show a blank line", labels.Type().Field(i).Name)
				}
			}
		})
	}

	if workerText("") != workerText("ru") {
		t.Error("a spec written before languages existed must still be shown in Russian")
	}
	if workerText("de") != workerText("en") {
		t.Error("an unknown language must fall back to English")
	}
	if workerText("ru") == workerText("en") {
		t.Error("Russian and English labels are identical")
	}

	en := workerText("en")
	if got := en.title("2.0.0"); !strings.HasSuffix(got, "2.0.0") || got == en.baseTitle {
		t.Errorf("title with a version = %q", got)
	}
	if got := en.title(""); got != en.baseTitle {
		t.Errorf("title without a version = %q, want %q", got, en.baseTitle)
	}
}

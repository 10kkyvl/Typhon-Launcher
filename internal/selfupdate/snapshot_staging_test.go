package selfupdate

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// stagedFixture has a committed snapshot of 1.2.0 and a staged one of 1.3.0,
// the state an update to 1.4.0 is in once the worker has copied the launcher.
type stagedFixture struct {
	rollbackFixture
}

func newStagedFixture(t *testing.T) stagedFixture {
	t.Helper()
	f := newRollbackFixture(t)
	if err := snapshotPrevious(context.Background(), f.configDir, f.target, "1.3.0"); err != nil {
		t.Fatalf("stage: %v", err)
	}
	return stagedFixture{f}
}

func (f stagedFixture) activeVersion(t *testing.T) string {
	t.Helper()
	rec, err := loadPrevious(f.configDir)
	if err != nil {
		t.Fatalf("loadPrevious: %v", err)
	}
	if _, err := verifyPreviousExe(context.Background(), f.configDir, rec); err != nil {
		t.Fatalf("the snapshot in use: %v", err)
	}
	return rec.Version
}

func (f stagedFixture) stagingLeft(t *testing.T) bool {
	t.Helper()
	cache := mustCacheDir(t, f.configDir)
	for _, name := range []string{stagingDirName, stagingRecordName} {
		if _, err := os.Stat(filepath.Join(cache, name)); err == nil {
			return true
		}
	}
	return false
}

func TestRecoverSnapshotSettlesAStagedCopy(t *testing.T) {
	cases := []struct {
		name string
		// damage plays the commit up to the moment the process died.
		damage  func(t *testing.T, f stagedFixture)
		current string
		want    string
	}{
		{name: "the update never happened", current: "1.3.0", want: "1.2.0"},
		{name: "the update failed, an older build runs", current: "1.2.5", want: "1.2.0"},
		{name: "the update went through", current: "1.4.0", want: "1.3.0"},
		{name: "the version is not comparable", current: "dev-build", want: "1.2.0"},
		{name: "died after dropping the old snapshot", current: "1.4.0", want: "1.3.0",
			damage: func(t *testing.T, f stagedFixture) {
				if err := dropPrevious(f.configDir); err != nil {
					t.Fatal(err)
				}
			}},
		{name: "died after moving the directory in, before the record", current: "1.4.0", want: "1.3.0",
			damage: func(t *testing.T, f stagedFixture) {
				if err := dropPrevious(f.configDir); err != nil {
					t.Fatal(err)
				}
				if err := os.Rename(mustStagingDir(t, f.configDir), mustPreviousDir(t, f.configDir)); err != nil {
					t.Fatal(err)
				}
			}},
		{name: "a record without any directory and no snapshot", current: "1.4.0", want: "",
			damage: func(t *testing.T, f stagedFixture) {
				if err := dropPrevious(f.configDir); err != nil {
					t.Fatal(err)
				}
				if err := os.RemoveAll(mustStagingDir(t, f.configDir)); err != nil {
					t.Fatal(err)
				}
			}},
		{name: "a directory without its record", current: "1.4.0", want: "1.2.0",
			damage: func(t *testing.T, f stagedFixture) {
				if err := os.Remove(mustStagingRecord(t, f.configDir)); err != nil {
					t.Fatal(err)
				}
			}},
		{name: "a damaged staged record", current: "1.4.0", want: "1.2.0",
			damage: func(t *testing.T, f stagedFixture) {
				writeTestFile(t, mustStagingRecord(t, f.configDir), []byte("{{{"))
			}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newStagedFixture(t)
			if tc.damage != nil {
				tc.damage(t, f)
			}

			if err := recoverSnapshot(f.configDir, tc.current); err != nil {
				t.Fatalf("recoverSnapshot: %v", err)
			}

			if f.stagingLeft(t) {
				t.Fatal("staging was left behind")
			}
			if tc.want == "" {
				if _, err := loadPrevious(f.configDir); !errors.Is(err, ErrNoPrevious) {
					t.Fatalf("loadPrevious = %v, want nothing on offer", err)
				}
				return
			}
			if got := f.activeVersion(t); got != tc.want {
				t.Fatalf("snapshot in use = %s, want %s", got, tc.want)
			}
		})
	}
}

func mustStagingDir(t *testing.T, configDir string) string {
	t.Helper()
	dir, err := stagingDir(configDir)
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

func mustStagingRecord(t *testing.T, configDir string) string {
	t.Helper()
	path, err := stagingRecordPath(configDir)
	if err != nil {
		t.Fatal(err)
	}
	return path
}

func TestCommitSnapshotReplacesTheSnapshotAndClearsTheFailureNote(t *testing.T) {
	f := newStagedFixture(t)
	if err := recordSnapshotFailure(f.configDir, "1.1.0", errors.New("earlier failure")); err != nil {
		t.Fatal(err)
	}
	if err := commitSnapshot(f.configDir); err != nil {
		t.Fatalf("commitSnapshot: %v", err)
	}
	if got := f.activeVersion(t); got != "1.3.0" {
		t.Fatalf("snapshot in use = %s", got)
	}
	if f.stagingLeft(t) {
		t.Fatal("staging survived the commit")
	}
	if _, found, err := loadSnapshotFailure(f.configDir); err != nil || found {
		t.Fatalf("failure note = %v, %v; a good snapshot clears it", found, err)
	}
	if err := commitSnapshot(f.configDir); err == nil {
		t.Fatal("a second commit with nothing staged = nil")
	}
}

func TestDiscardStagingKeepsTheSnapshotInUse(t *testing.T) {
	f := newStagedFixture(t)
	if err := discardStaging(f.configDir); err != nil {
		t.Fatal(err)
	}
	if f.stagingLeft(t) {
		t.Fatal("staging survived")
	}
	if got := f.activeVersion(t); got != "1.2.0" {
		t.Fatalf("snapshot in use = %s", got)
	}
	if err := discardStaging(f.configDir); err != nil {
		t.Fatalf("discarding nothing: %v", err)
	}
}

func TestSweepPreviousTempsRemovesOnlyOldTempFilesInsideTheSnapshots(t *testing.T) {
	f := newStagedFixture(t)
	prev := mustPreviousDir(t, f.configDir)
	stage := mustStagingDir(t, f.configDir)
	oldTemp := filepath.Join(prev, previousStateName, ".tmp-1")
	oldStaged := filepath.Join(stage, ".tmp-2")
	young := filepath.Join(prev, ".tmp-3")
	keep := filepath.Join(prev, "notes.txt")
	outside := filepath.Join(mustCacheDir(t, f.configDir), ".tmp-4")
	for _, p := range []string{oldTemp, oldStaged, young, keep, outside} {
		writeTestFile(t, p, []byte("x"))
	}
	ageFile(t, oldTemp, 2*previousTempAge)
	ageFile(t, oldStaged, 2*previousTempAge)
	ageFile(t, outside, 2*previousTempAge)
	ageFile(t, young, previousTempAge/4)

	sweepPreviousTemps(f.configDir)

	for _, gone := range []string{oldTemp, oldStaged} {
		if _, err := os.Stat(gone); !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("%s survived: %v", gone, err)
		}
	}
	for _, kept := range []string{young, keep, outside} {
		if _, err := os.Stat(kept); err != nil {
			t.Fatalf("%s was removed: %v", kept, err)
		}
	}
	sweepPreviousTemps(filepath.Join(t.TempDir(), "Typhon"))
}

func TestRecoverInterruptedSettlesBothAtOnce(t *testing.T) {
	f := newStagedFixture(t)
	seedEnveloped(t, f.configDir, "second.json", 1, "old")
	if err := RecoverInterrupted(f.configDir, "1.4.0"); err != nil {
		t.Fatalf("RecoverInterrupted: %v", err)
	}
	if got := f.activeVersion(t); got != "1.3.0" {
		t.Fatalf("snapshot in use = %s, want the staged one promoted", got)
	}
}

func TestSnapshotFailureNote(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), "Typhon")
	if _, found, err := loadSnapshotFailure(cfg); err != nil || found {
		t.Fatalf("no note yet = %v, %v", found, err)
	}
	if err := recordSnapshotFailure(cfg, "1.2.0", errors.New("catalog.json: access denied")); err != nil {
		t.Fatal(err)
	}
	note, found, err := loadSnapshotFailure(cfg)
	if err != nil || !found {
		t.Fatalf("note = %v, %v", found, err)
	}
	if note.Version != "1.2.0" || note.Reason != "catalog.json: access denied" || time.Since(note.At) > time.Minute {
		t.Fatalf("note = %+v", note)
	}
	cache := mustCacheDir(t, cfg)
	writeTestFile(t, filepath.Join(cache, snapshotFailedName), []byte("{{{"))
	if _, _, err := loadSnapshotFailure(cfg); err == nil {
		t.Fatal("a damaged note read as no note")
	}
}

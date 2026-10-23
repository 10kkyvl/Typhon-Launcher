package selfupdate

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"typhon/internal/storage"
)

// replacedFixture is a rollback caught between its steps: two state files are
// migrated, their live content is what the new launcher wrote, and the
// functions below play the steps one at a time.
type replacedFixture struct {
	rollbackFixture
	items  []restoreItem
	before map[string]string
}

func newReplacedFixture(t *testing.T) replacedFixture {
	t.Helper()
	f := newRollbackFixture(t, func(cfg string) { seedEnveloped(t, cfg, "second.json", 1, "old") })
	seedEnveloped(t, f.configDir, "second.json", 2, "migrated")
	rec, err := loadPrevious(f.configDir)
	if err != nil {
		t.Fatal(err)
	}
	items, err := planStateRestore(context.Background(), f.configDir, rec)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("setup: %d files to restore, want 2", len(items))
	}
	return replacedFixture{rollbackFixture: f, items: items, before: liveState(t, f.configDir)}
}

func TestRecoverReplacedAfterTheWorkerDied(t *testing.T) {
	cases := []struct {
		name string
		// steps plays the rollback up to the moment the worker died.
		steps   func(t *testing.T, f replacedFixture)
		current string
		// wantBefore: the live files must be exactly as before the rollback.
		wantBefore bool
		wantErr    bool
		wantIs     error
		keptLeft   bool
	}{
		{name: "died after the copies and the journal, before any overwrite",
			steps: func(t *testing.T, f replacedFixture) {
				mustSaveReplaced(t, f)
			}, current: "1.3.0", wantBefore: true},
		{name: "died after the first overwrite",
			steps: func(t *testing.T, f replacedFixture) {
				mustSaveReplaced(t, f)
				if err := writeRestored(context.Background(), f.items[:1]); err != nil {
					t.Fatal(err)
				}
			}, current: "1.3.0", wantBefore: true},
		{name: "died after every overwrite, before the swap",
			steps: func(t *testing.T, f replacedFixture) {
				mustSaveReplaced(t, f)
				if err := writeRestored(context.Background(), f.items); err != nil {
					t.Fatal(err)
				}
			}, current: "v1.3.0", wantBefore: true},
		{name: "died while the copies were still being made",
			steps: func(t *testing.T, f replacedFixture) {
				dir := mustReplacedDir(t, f.configDir)
				writeTestFile(t, filepath.Join(dir, "playlog.json"), []byte("half a copy"))
			}, current: "1.3.0", wantBefore: true},
		{name: "swap had happened, died before dropping the snapshot",
			steps: func(t *testing.T, f replacedFixture) {
				mustSaveReplaced(t, f)
				if err := writeRestored(context.Background(), f.items); err != nil {
					t.Fatal(err)
				}
			}, current: "1.2.0", wantBefore: false},
		{name: "running a third version",
			steps: func(t *testing.T, f replacedFixture) {
				mustSaveReplaced(t, f)
				if err := writeRestored(context.Background(), f.items[:1]); err != nil {
					t.Fatal(err)
				}
			}, current: "1.9.0", wantBefore: false},
		{name: "a kept copy was damaged",
			steps: func(t *testing.T, f replacedFixture) {
				mustSaveReplaced(t, f)
				if err := writeRestored(context.Background(), f.items); err != nil {
					t.Fatal(err)
				}
				writeTestFile(t, filepath.Join(mustReplacedDir(t, f.configDir), "second.json"), []byte("rotten"))
			}, current: "1.3.0", wantErr: true, wantIs: ErrPreviousStateHash, keptLeft: true},
		{name: "the journal is unreadable",
			steps: func(t *testing.T, f replacedFixture) {
				mustSaveReplaced(t, f)
				writeTestFile(t, filepath.Join(mustReplacedDir(t, f.configDir), replacedJournal), []byte("{{{"))
			}, current: "1.3.0", wantErr: true, keptLeft: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newReplacedFixture(t)
			tc.steps(t, f)

			err := recoverReplaced(f.configDir, tc.current)

			if tc.wantErr {
				if err == nil {
					t.Fatal("recoverReplaced() error = nil")
				}
				if tc.wantIs != nil && !errors.Is(err, tc.wantIs) {
					t.Fatalf("error = %v, want %v", err, tc.wantIs)
				}
			} else if err != nil {
				t.Fatalf("recoverReplaced: %v", err)
			}
			_, statErr := os.Stat(mustReplacedDir(t, f.configDir))
			if tc.keptLeft != (statErr == nil) {
				t.Fatalf("kept copies present = %v, want %v (%v)", statErr == nil, tc.keptLeft, statErr)
			}
			if tc.wantBefore {
				assertSameState(t, f.before, liveState(t, f.configDir))
			}
		})
	}
}

func mustSaveReplaced(t *testing.T, f replacedFixture) {
	t.Helper()
	if err := saveReplaced(context.Background(), f.configDir, f.items, "1.3.0", "1.2.0"); err != nil {
		t.Fatalf("saveReplaced: %v", err)
	}
}

func TestRecoverReplacedWithNothingToDoIsQuiet(t *testing.T) {
	f := newRollbackFixture(t)
	if err := recoverReplaced(f.configDir, "1.3.0"); err != nil {
		t.Fatalf("recoverReplaced: %v", err)
	}
	if err := recoverReplaced(filepath.Join(t.TempDir(), "Typhon"), "1.3.0"); err != nil {
		t.Fatalf("recoverReplaced without a cache: %v", err)
	}
}

func TestSaveReplacedKeepsWhatTheLiveFilesHold(t *testing.T) {
	f := newReplacedFixture(t)
	mustSaveReplaced(t, f)

	var rec replacedRecord
	dir := mustReplacedDir(t, f.configDir)
	if err := storage.Load(filepath.Join(dir, replacedJournal), replacedJournalVer, nil, &rec); err != nil {
		t.Fatal(err)
	}
	if rec.From != "1.3.0" || rec.To != "1.2.0" || len(rec.Files) != 2 {
		t.Fatalf("journal = %+v", rec)
	}
	for _, file := range rec.Files {
		kept := readTestFile(t, filepath.Join(dir, filepath.FromSlash(file.Path)))
		if kept != f.before[file.Path] {
			t.Fatalf("kept copy of %s is not what the live file held", file.Path)
		}
		if sha256Hex(t, []byte(kept)) != file.SHA256 || int64(len(kept)) != file.Size {
			t.Fatalf("journal entry for %s does not match the kept copy", file.Path)
		}
	}
}

func TestSaveReplacedCancelledWritesNoJournal(t *testing.T) {
	f := newReplacedFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := saveReplaced(ctx, f.configDir, f.items, "1.3.0", "1.2.0"); !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
	if _, err := os.Stat(filepath.Join(mustReplacedDir(t, f.configDir), replacedJournal)); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("a journal exists for copies that were never made: %v", err)
	}
	assertSameState(t, f.before, liveState(t, f.configDir))
}

func TestReplacedRecordValidate(t *testing.T) {
	good := replacedFile{Path: "a/b.json", SHA256: sha256Hex(t, []byte("x")), Size: 1}
	cases := []struct {
		name string
		rec  replacedRecord
		ok   bool
	}{
		{"valid", replacedRecord{From: "1.3.0", To: "1.2.0", Files: []replacedFile{good}}, true},
		{"no versions", replacedRecord{Files: []replacedFile{good}}, false},
		{"path escapes", replacedRecord{From: "1", To: "2", Files: []replacedFile{{Path: "../x.json", SHA256: good.SHA256}}}, false},
		{"path is not json", replacedRecord{From: "1", To: "2", Files: []replacedFile{{Path: "x.exe", SHA256: good.SHA256}}}, false},
		{"bad hash", replacedRecord{From: "1", To: "2", Files: []replacedFile{{Path: "x.json", SHA256: "zz"}}}, false},
	}
	for _, tc := range cases {
		if err := tc.rec.validate(); (err == nil) != tc.ok {
			t.Errorf("%s: validate() = %v, want ok = %v", tc.name, err, tc.ok)
		}
	}
}

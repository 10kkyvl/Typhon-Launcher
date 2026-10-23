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

func statePaths(rec previousRecord) []string {
	paths := make([]string, 0, len(rec.StateBackup))
	for _, f := range rec.StateBackup {
		paths = append(paths, f.Path)
	}
	return paths
}

func TestSnapshotPreviousSavesTheLauncherAndOnlyVersionedState(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), "Typhon")
	exe := filepath.Join(t.TempDir(), "typhon.exe")
	writeTestFile(t, exe, []byte("launcher 1.2.0"))

	seedEnveloped(t, cfg, "playlog.json", 2, []string{"a"})
	seedEnveloped(t, cfg, "catalog-pages/p1.json", 4, []string{"page"})
	writeTestFile(t, filepath.Join(cfg, "library.json"), []byte(`[{"id":"g1"}]`))
	writeTestFile(t, filepath.Join(cfg, "settings.json"), []byte(`{"theme":"dark"}`))
	writeTestFile(t, filepath.Join(cfg, "broken.json"), []byte(`{"version":1,"data":{`))
	writeTestFile(t, filepath.Join(cfg, "typhon.log"), []byte("log"))
	seedEnveloped(t, cfg, "selfupdate/state.json", 1, "updater")
	seedEnveloped(t, cfg, "webview2/EBWebView/prefs.json", 1, "profile")
	seedEnveloped(t, cfg, "nested/EBWebView/prefs.json", 1, "profile")
	seedEnveloped(t, cfg, "broker-1/pin.json", 1, "pin")
	seedEnveloped(t, cfg, "worker-abc-spec.json", 1, "spec")
	seedEnveloped(t, cfg, "save-backups/s/index.json", 1, "saves")
	seedEnveloped(t, cfg, "diagnostics/pending/x.json", 1, "diag")

	if err := snapshotPrevious(context.Background(), cfg, exe, "1.2.0"); err != nil {
		t.Fatalf("snapshotPrevious: %v", err)
	}
	if _, err := loadPrevious(cfg); !errors.Is(err, ErrNoPrevious) {
		t.Fatalf("a snapshot that is only staged is on offer: %v", err)
	}
	if err := commitSnapshot(cfg); err != nil {
		t.Fatalf("commitSnapshot: %v", err)
	}

	rec, err := loadPrevious(cfg)
	if err != nil {
		t.Fatalf("loadPrevious: %v", err)
	}
	if rec.Version != "1.2.0" || rec.Size != int64(len("launcher 1.2.0")) || rec.SHA256 != sha256Hex(t, []byte("launcher 1.2.0")) {
		t.Fatalf("record = %+v", rec)
	}
	if rec.SavedAt.IsZero() {
		t.Fatal("record has no save time")
	}
	want := []string{"catalog-pages/p1.json", "playlog.json"}
	if got := statePaths(rec); !reflect.DeepEqual(got, want) {
		t.Fatalf("saved state files = %v, want %v", got, want)
	}
	if rec.StateBackup[1].Version != 2 || rec.StateBackup[0].Version != 4 {
		t.Fatalf("envelope versions not recorded: %+v", rec.StateBackup)
	}

	dir, err := PreviousDir(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if got := readTestFile(t, filepath.Join(dir, previousExeName)); got != "launcher 1.2.0" {
		t.Fatalf("saved launcher = %q", got)
	}
	for _, f := range rec.StateBackup {
		saved := readTestFile(t, filepath.Join(dir, previousStateName, filepath.FromSlash(f.Path)))
		if got := sha256Hex(t, []byte(saved)); got != f.SHA256 || int64(len(saved)) != f.Size {
			t.Fatalf("saved copy of %s differs from its record", f.Path)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, previousStateName, "library.json")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("a file without an envelope was copied: %v", err)
	}
}

// A snapshot that cannot be made never costs the user the one they have:
// the new copy is built beside it and thrown away on any failure.
func TestSnapshotPreviousFailuresKeepTheEarlierSnapshot(t *testing.T) {
	cases := []struct {
		name   string
		ver    string
		exe    func(t *testing.T) string
		want   error
		cancel bool
	}{
		{name: "empty version", ver: "", exe: okExe, want: ErrSnapshotVersion},
		{name: "version is not a path segment", ver: "1.2/0", exe: okExe, want: ErrSnapshotVersion},
		{name: "launcher is missing", ver: "1.2.0", exe: func(t *testing.T) string { return filepath.Join(t.TempDir(), "gone.exe") }},
		{name: "launcher path is empty", ver: "1.2.0", exe: func(*testing.T) string { return "" }},
		{name: "launcher is a directory", ver: "1.2.0", exe: func(t *testing.T) string { return t.TempDir() }},
		{name: "cancelled context", ver: "1.2.0", exe: okExe, cancel: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := filepath.Join(t.TempDir(), "Typhon")
			seedEnveloped(t, cfg, "playlog.json", 2, "a")
			if err := snapshotPrevious(context.Background(), cfg, okExe(t), "1.1.0"); err != nil {
				t.Fatalf("first snapshot: %v", err)
			}
			if err := commitSnapshot(cfg); err != nil {
				t.Fatal(err)
			}

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if tc.cancel {
				cancel()
			}
			err := snapshotPrevious(ctx, cfg, tc.exe(t), tc.ver)
			if err == nil {
				t.Fatal("snapshotPrevious() error = nil")
			}
			if tc.want != nil && !errors.Is(err, tc.want) {
				t.Fatalf("error = %v, want %v", err, tc.want)
			}
			rec, loadErr := loadPrevious(cfg)
			if loadErr != nil || rec.Version != "1.1.0" {
				t.Fatalf("the earlier snapshot after a failed one = %+v, %v", rec, loadErr)
			}
			if _, err := verifyPreviousExe(context.Background(), cfg, rec); err != nil {
				t.Fatalf("the earlier snapshot's launcher: %v", err)
			}
			staging := mustCacheDir(t, cfg)
			for _, name := range []string{stagingDirName, stagingRecordName} {
				if _, err := os.Stat(filepath.Join(staging, name)); !errors.Is(err, fs.ErrNotExist) {
					t.Fatalf("%s left behind: %v", name, err)
				}
			}
		})
	}
}

func okExe(t *testing.T) string {
	t.Helper()
	exe := filepath.Join(t.TempDir(), "typhon.exe")
	writeTestFile(t, exe, []byte("launcher"))
	return exe
}

func TestSnapshotPreviousReplacesTheEarlierSnapshotWhole(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), "Typhon")
	exe := okExe(t)
	seedEnveloped(t, cfg, "old-only.json", 1, "x")
	seedEnveloped(t, cfg, "kept.json", 1, "x")
	if err := snapshotPrevious(context.Background(), cfg, exe, "1.1.0"); err != nil {
		t.Fatal(err)
	}
	if err := commitSnapshot(cfg); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(cfg, "old-only.json")); err != nil {
		t.Fatal(err)
	}
	if err := snapshotPrevious(context.Background(), cfg, exe, "1.2.0"); err != nil {
		t.Fatal(err)
	}
	if rec, err := loadPrevious(cfg); err != nil || rec.Version != "1.1.0" {
		t.Fatalf("before the commit the snapshot in use is %+v, %v; want the earlier one", rec, err)
	}
	if err := commitSnapshot(cfg); err != nil {
		t.Fatal(err)
	}

	rec, err := loadPrevious(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if rec.Version != "1.2.0" || !reflect.DeepEqual(statePaths(rec), []string{"kept.json"}) {
		t.Fatalf("record = %+v", rec)
	}
	dir := mustPreviousDir(t, cfg)
	if _, err := os.Stat(filepath.Join(dir, previousStateName, "old-only.json")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("a file of the earlier snapshot survived: %v", err)
	}
}

func TestSnapshotPreviousWalksIntoDirectoriesNamedLikeJSONFiles(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), "Typhon")
	seedEnveloped(t, cfg, "playlog.json", 2, "a")
	seedEnveloped(t, cfg, "dir.json/inner/x.json", 3, "b")

	if err := snapshotPrevious(context.Background(), cfg, okExe(t), "1.2.0"); err != nil {
		t.Fatalf("snapshotPrevious: %v", err)
	}
	if err := commitSnapshot(cfg); err != nil {
		t.Fatal(err)
	}
	rec, err := loadPrevious(cfg)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"dir.json/inner/x.json", "playlog.json"}
	if got := statePaths(rec); !reflect.DeepEqual(got, want) {
		t.Fatalf("saved = %v, want %v", got, want)
	}
}

func TestLoadPrevious(t *testing.T) {
	validHash := strings.Repeat("ab", 32)
	valid := previousRecord{Version: "1.2.0", SHA256: validHash, Size: 10, StateBackup: []stateFile{{Path: "a/b.json", Version: 1, SHA256: validHash, Size: 4}}}
	mutate := func(fn func(r *previousRecord)) previousRecord {
		r := valid
		r.StateBackup = append([]stateFile(nil), valid.StateBackup...)
		fn(&r)
		return r
	}
	cases := []struct {
		name    string
		write   func(t *testing.T, path string)
		want    error
		wantRec bool
	}{
		{name: "no record", write: func(*testing.T, string) {}, want: ErrNoPrevious},
		{name: "not json", write: func(t *testing.T, p string) { writeTestFile(t, p, []byte("{{{")) }, want: ErrPreviousBroken},
		{name: "empty file", write: func(t *testing.T, p string) { writeTestFile(t, p, nil) }, want: ErrPreviousBroken},
		{name: "envelope newer than this build", write: func(t *testing.T, p string) {
			if err := storage.Save(p, previousRecordVersion+1, valid); err != nil {
				t.Fatal(err)
			}
		}, want: ErrPreviousBroken},
		{name: "empty payload", write: func(t *testing.T, p string) {
			if err := storage.Save(p, previousRecordVersion, nil); err != nil {
				t.Fatal(err)
			}
		}, want: ErrPreviousBroken},
		{name: "valid", write: saveRecord(valid), wantRec: true},
		{name: "no version", write: saveRecord(mutate(func(r *previousRecord) { r.Version = "" })), want: ErrPreviousBroken},
		{name: "version with a separator", write: saveRecord(mutate(func(r *previousRecord) { r.Version = "1/2" })), want: ErrPreviousBroken},
		{name: "bad launcher hash", write: saveRecord(mutate(func(r *previousRecord) { r.SHA256 = "xyz" })), want: ErrPreviousBroken},
		{name: "no launcher size", write: saveRecord(mutate(func(r *previousRecord) { r.Size = 0 })), want: ErrPreviousBroken},
		{name: "state path escapes", write: saveRecord(mutate(func(r *previousRecord) { r.StateBackup[0].Path = "../settings.json" })), want: ErrPreviousBroken},
		{name: "state path is absolute", write: saveRecord(mutate(func(r *previousRecord) { r.StateBackup[0].Path = "/etc/x.json" })), want: ErrPreviousBroken},
		{name: "state path is not clean", write: saveRecord(mutate(func(r *previousRecord) { r.StateBackup[0].Path = "a/./b.json" })), want: ErrPreviousBroken},
		{name: "state path is not json", write: saveRecord(mutate(func(r *previousRecord) { r.StateBackup[0].Path = "a/b.exe" })), want: ErrPreviousBroken},
		{name: "state path listed twice", write: saveRecord(mutate(func(r *previousRecord) { r.StateBackup = append(r.StateBackup, r.StateBackup[0]) })), want: ErrPreviousBroken},
		{name: "state version zero", write: saveRecord(mutate(func(r *previousRecord) { r.StateBackup[0].Version = 0 })), want: ErrPreviousBroken},
		{name: "state hash bad", write: saveRecord(mutate(func(r *previousRecord) { r.StateBackup[0].SHA256 = "00" })), want: ErrPreviousBroken},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := filepath.Join(t.TempDir(), "Typhon")
			path, err := previousRecordPath(cfg)
			if err != nil {
				t.Fatal(err)
			}
			tc.write(t, path)

			rec, err := loadPrevious(cfg)
			if tc.wantRec {
				if err != nil || rec.Version != "1.2.0" || len(rec.StateBackup) != 1 {
					t.Fatalf("loadPrevious = %+v, %v", rec, err)
				}
				return
			}
			if !errors.Is(err, tc.want) {
				t.Fatalf("error = %v, want %v", err, tc.want)
			}
			if errors.Is(tc.want, ErrNoPrevious) && errors.Is(err, ErrPreviousBroken) {
				t.Fatal("a missing record is not a damaged one")
			}
		})
	}
}

func saveRecord(rec previousRecord) func(t *testing.T, path string) {
	return func(t *testing.T, path string) {
		t.Helper()
		if err := storage.Save(path, previousRecordVersion, rec); err != nil {
			t.Fatal(err)
		}
	}
}

func TestLoadPreviousCacheDirIsRequired(t *testing.T) {
	if _, err := loadPrevious(""); !errors.Is(err, ErrEmptyConfigDir) {
		t.Fatalf("error = %v, want ErrEmptyConfigDir", err)
	}
	if err := snapshotPrevious(context.Background(), "", okExe(t), "1.2.0"); !errors.Is(err, ErrEmptyConfigDir) {
		t.Fatalf("snapshot error = %v, want ErrEmptyConfigDir", err)
	}
}

func TestVerifyPreviousExe(t *testing.T) {
	cases := []struct {
		name   string
		damage func(t *testing.T, exe string)
		want   error
	}{
		{name: "intact", damage: func(*testing.T, string) {}},
		{name: "missing", damage: func(t *testing.T, exe string) {
			if err := os.Remove(exe); err != nil {
				t.Fatal(err)
			}
		}, want: ErrPreviousBroken},
		{name: "same size, other bytes", damage: func(t *testing.T, exe string) { writeTestFile(t, exe, []byte("launcher 1.2.1")) }, want: ErrPreviousHash},
		{name: "truncated", damage: func(t *testing.T, exe string) { writeTestFile(t, exe, []byte("launcher")) }, want: ErrPreviousHash},
		{name: "a directory", damage: func(t *testing.T, exe string) {
			if err := os.Remove(exe); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(exe, 0o700); err != nil {
				t.Fatal(err)
			}
		}, want: ErrPreviousHash},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newRollbackFixture(t)
			rec, err := loadPrevious(f.configDir)
			if err != nil {
				t.Fatal(err)
			}
			dir := mustPreviousDir(t, f.configDir)
			exe := filepath.Join(dir, previousExeName)
			tc.damage(t, exe)

			got, err := verifyPreviousExe(context.Background(), f.configDir, rec)
			if tc.want == nil {
				if err != nil || got != exe {
					t.Fatalf("verifyPreviousExe = %q, %v", got, err)
				}
				return
			}
			if !errors.Is(err, tc.want) {
				t.Fatalf("error = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestPlanStateRestoreTakesOnlyTheFilesTheUpdateMigrated(t *testing.T) {
	f := newRollbackFixture(t, func(cfg string) { seedEnveloped(t, cfg, "corrupt.json", 1, "x") })
	// Live state after the update: playlog migrated, history grew but kept
	// its version, the catalog page is gone, corrupt.json is garbage now.
	if err := os.Remove(filepath.Join(f.configDir, "catalog-pages", "p1.json")); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(f.configDir, "corrupt.json"), []byte("not json at all"))

	rec, err := loadPrevious(f.configDir)
	if err != nil {
		t.Fatal(err)
	}
	items, err := planStateRestore(context.Background(), f.configDir, rec)
	if err != nil {
		t.Fatalf("planStateRestore: %v", err)
	}
	var got []string
	for _, it := range items {
		rel, err := filepath.Rel(f.configDir, it.dst)
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, filepath.ToSlash(rel))
	}
	want := []string{"corrupt.json", "playlog.json"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("restoring %v, want %v: history.json is at its saved version (newer data stays), catalog-pages/p1.json is gone (stays gone)", got, want)
	}

	if err := writeRestored(context.Background(), items); err != nil {
		t.Fatalf("writeRestored: %v", err)
	}
	if v := configEnvelopeVersion(t, f.configDir, "playlog.json"); v != 2 {
		t.Fatalf("playlog.json envelope version = %d, want the saved 2", v)
	}
	if !strings.Contains(readConfigFile(t, f.configDir, "history.json"), "entry-b") {
		t.Fatal("history.json lost the entry added after the update")
	}
	if _, err := os.Stat(filepath.Join(f.configDir, "catalog-pages", "p1.json")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("a file that was gone came back: %v", err)
	}
}

func TestPlanStateRestoreRefusesADamagedBackup(t *testing.T) {
	cases := []struct {
		name   string
		damage func(t *testing.T, saved string)
		want   error
	}{
		{name: "changed bytes", damage: func(t *testing.T, saved string) { writeTestFile(t, saved, []byte(`{"version":2,"data":["tampered"]}`)) }, want: ErrPreviousStateHash},
		{name: "backup file gone", damage: func(t *testing.T, saved string) {
			if err := os.Remove(saved); err != nil {
				t.Fatal(err)
			}
		}, want: ErrPreviousBroken},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newRollbackFixture(t)
			dir := mustPreviousDir(t, f.configDir)
			tc.damage(t, filepath.Join(dir, previousStateName, "playlog.json"))
			rec, err := loadPrevious(f.configDir)
			if err != nil {
				t.Fatal(err)
			}

			items, err := planStateRestore(context.Background(), f.configDir, rec)
			if !errors.Is(err, tc.want) {
				t.Fatalf("error = %v, want %v", err, tc.want)
			}
			if items != nil {
				t.Fatalf("a damaged backup produced a plan: %d items", len(items))
			}
		})
	}
}

func TestPlanStateRestoreHonoursCancellation(t *testing.T) {
	f := newRollbackFixture(t)
	rec, err := loadPrevious(f.configDir)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := planStateRestore(ctx, f.configDir, rec); !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
	if err := writeRestored(ctx, []restoreItem{{rel: "x.json", dst: filepath.Join(f.configDir, "x.json"), data: []byte("x")}}); !errors.Is(err, context.Canceled) {
		t.Fatalf("writeRestored error = %v, want context.Canceled", err)
	}
}

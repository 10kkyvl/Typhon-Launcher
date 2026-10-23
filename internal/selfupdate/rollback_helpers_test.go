package selfupdate

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"typhon/internal/storage"
)

// stubSaveCurrent keeps the worker from copying the test binary's state: the
// tests of the worker's orchestration are not about the snapshot itself.
func stubSaveCurrent(t *testing.T) {
	t.Helper()
	restoreSave, restoreCommit := saveCurrent, commitCurrent
	saveCurrent = func(context.Context, string, string, string) error { return nil }
	commitCurrent = func(string) error { return nil }
	t.Cleanup(func() { saveCurrent, commitCurrent = restoreSave, restoreCommit })
}

func stubRollbackSupported(t *testing.T, err error) {
	t.Helper()
	restore := rollbackSupported
	rollbackSupported = func() error { return err }
	t.Cleanup(func() { rollbackSupported = restore })
}

type displayVersionCalls struct {
	set      []string
	reverted int
	err      error
}

// stubDisplayVersion replaces the uninstall-entry write; a run must never
// touch the registry of the machine it runs on.
func stubDisplayVersion(t *testing.T) *displayVersionCalls {
	t.Helper()
	calls := &displayVersionCalls{}
	restore := updateDisplayVersion
	updateDisplayVersion = func(ver string) (func() error, error) {
		if calls.err != nil {
			return nil, calls.err
		}
		calls.set = append(calls.set, ver)
		return func() error { calls.reverted++; return nil }, nil
	}
	t.Cleanup(func() { updateDisplayVersion = restore })
	return calls
}

func seedEnveloped(t *testing.T, configDir, rel string, version int, data any) {
	t.Helper()
	if err := storage.Save(filepath.Join(configDir, filepath.FromSlash(rel)), version, data); err != nil {
		t.Fatalf("seed %s: %v", rel, err)
	}
}

func readConfigFile(t *testing.T, configDir, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(configDir, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return string(data)
}

func readTestFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

func ageFile(t *testing.T, path string, age time.Duration) {
	t.Helper()
	when := time.Now().Add(-age)
	if err := os.Chtimes(path, when, when); err != nil {
		t.Fatalf("age %s: %v", path, err)
	}
}

// rollbackFixture is a launcher directory with a saved previous version: the
// state the worker finds when a rollback starts.
type rollbackFixture struct {
	configDir string
	target    string
	oldExe    string
	newExe    string
}

// newRollbackFixture saves version 1.2.0 with three state files, then plays an
// update to 1.3.0 that migrates playlog.json to a newer envelope version and
// leaves history.json at its version while the user keeps adding to it.
func newRollbackFixture(t *testing.T, beforeSnapshot ...func(configDir string)) rollbackFixture {
	t.Helper()
	return newRollbackFixtureAt(t, filepath.Join(t.TempDir(), "Typhon"), beforeSnapshot...)
}

func newRollbackFixtureAt(t *testing.T, configDir string, beforeSnapshot ...func(configDir string)) rollbackFixture {
	t.Helper()
	f := rollbackFixture{
		configDir: configDir,
		oldExe:    "launcher 1.2.0",
		newExe:    "launcher 1.3.0, a different build",
	}
	installDir := t.TempDir()
	f.target = filepath.Join(installDir, "typhon.exe")
	writeTestFile(t, f.target, []byte(f.oldExe))

	seedEnveloped(t, f.configDir, "playlog.json", 2, []string{"session-a"})
	seedEnveloped(t, f.configDir, "history.json", 1, []string{"entry-a"})
	seedEnveloped(t, f.configDir, "catalog-pages/p1.json", 4, []string{"page"})
	writeTestFile(t, filepath.Join(f.configDir, "library.json"), []byte(`[{"id":"g1"}]`))

	for _, seed := range beforeSnapshot {
		seed(f.configDir)
	}
	if err := snapshotPrevious(context.Background(), f.configDir, f.target, "1.2.0"); err != nil {
		t.Fatalf("snapshotPrevious: %v", err)
	}
	if err := commitSnapshot(f.configDir); err != nil {
		t.Fatalf("commitSnapshot: %v", err)
	}

	writeTestFile(t, f.target, []byte(f.newExe))
	seedEnveloped(t, f.configDir, "playlog.json", 3, map[string]any{"sessions": []string{"session-a", "session-b"}})
	seedEnveloped(t, f.configDir, "history.json", 1, []string{"entry-a", "entry-b"})
	return f
}

func mustPreviousDir(t *testing.T, configDir string) string {
	t.Helper()
	dir, err := PreviousDir(configDir)
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

func mustPreviousRecordPath(t *testing.T, configDir string) string {
	t.Helper()
	path, err := previousRecordPath(configDir)
	if err != nil {
		t.Fatal(err)
	}
	return path
}

func mustCacheDir(t *testing.T, configDir string) string {
	t.Helper()
	dir, err := CacheDir(configDir)
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

func mustWorkerDir(t *testing.T, configDir string) string {
	t.Helper()
	dir, err := WorkerDir(configDir)
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

// configEnvelopeVersion is the envelope version of a state file; 0 for a file
// that has none.
func configEnvelopeVersion(t *testing.T, configDir, rel string) int {
	t.Helper()
	version, _, err := storage.EnvelopeVersion([]byte(readConfigFile(t, configDir, rel)))
	if err != nil {
		t.Fatalf("%s: %v", rel, err)
	}
	return version
}

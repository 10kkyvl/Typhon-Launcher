package selfupdate

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// TestApplyUpdateStartsTheWorkerFromACopy closes the root cause of "the
// launcher closed and nothing happened": the worker used to be started from
// the launcher binary itself, which is the very file the installer has to
// overwrite. Windows keeps a running image locked, so NSIS skipped the file
// and still exited 0, and the relaunch brought back the old version.
func TestApplyUpdateStartsTheWorkerFromACopy(t *testing.T) {
	dir := t.TempDir()
	store := mustStore(t, dir)
	readyPath := filepath.Join(dir, "selfupdate", "1.2.3", "setup.exe")
	writeTestFile(t, readyPath, []byte("installer"))

	s := &Service{dir: dir, notes: mustNotesStore(t, dir), store: store, currentVersion: "1.0.0", readyPath: readyPath}
	s.status = Status{State: StateReady, CurrentVersion: "1.0.0", AvailableVersion: "1.2.3"}

	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("Executable: %v", err)
	}

	var workerPath, specPath string
	restore := startWorker
	startWorker = func(worker, spec string) error {
		workerPath, specPath = worker, spec
		return nil
	}
	t.Cleanup(func() { startWorker = restore })

	if err := s.ApplyUpdate(); err != nil {
		t.Fatalf("ApplyUpdate() error = %v, want nil", err)
	}

	if workerPath == "" {
		t.Fatal("no worker was started")
	}
	if workerPath == exe {
		t.Fatalf("worker was started from %q, the binary the installer has to overwrite", exe)
	}
	workerDir, err := WorkerDir(dir)
	if err != nil {
		t.Fatalf("WorkerDir: %v", err)
	}
	if filepath.Dir(workerPath) != workerDir {
		t.Fatalf("worker copy at %q, want it inside %q", workerPath, workerDir)
	}

	original, err := os.ReadFile(exe)
	if err != nil {
		t.Fatalf("read launcher binary: %v", err)
	}
	copied, err := os.ReadFile(workerPath)
	if err != nil {
		t.Fatalf("read worker copy: %v", err)
	}
	if !bytes.Equal(original, copied) {
		t.Fatal("worker copy differs from the binary it was copied from")
	}

	spec, err := readUpdateSpec(specPath)
	if err != nil {
		t.Fatalf("readUpdateSpec: %v", err)
	}
	if spec.RelaunchPath != exe {
		t.Fatalf("spec.RelaunchPath = %q, want the launcher itself %q", spec.RelaunchPath, exe)
	}
	// Without this the silent installer falls back to the directory compiled
	// into it and installs beside the running launcher instead of over it.
	if spec.InstallDir != filepath.Dir(exe) {
		t.Fatalf("spec.InstallDir = %q, want the directory the launcher runs from %q", spec.InstallDir, filepath.Dir(exe))
	}
	if spec.Version != "1.2.3" {
		t.Fatalf("spec.Version = %q, want %q: the worker names the version on screen", spec.Version, "1.2.3")
	}
	if spec.InstallerPath != readyPath {
		t.Fatalf("spec.InstallerPath = %q, want %q", spec.InstallerPath, readyPath)
	}
	if spec.ParentPID != os.Getpid() {
		t.Fatalf("spec.ParentPID = %d, want %d", spec.ParentPID, os.Getpid())
	}
}

func TestApplyUpdateRollsBackWhenTheWorkerCannotStart(t *testing.T) {
	dir := t.TempDir()
	s := &Service{dir: dir, notes: mustNotesStore(t, dir), store: mustStore(t, dir), currentVersion: "1.0.0", readyPath: filepath.Join(dir, "setup.exe")}
	s.status = Status{State: StateReady, CurrentVersion: "1.0.0", AvailableVersion: "1.2.3"}

	wantErr := errors.New("worker refused to start")
	restore := startWorker
	startWorker = func(string, string) error { return wantErr }
	t.Cleanup(func() { startWorker = restore })

	if err := s.ApplyUpdate(); !errors.Is(err, wantErr) {
		t.Fatalf("ApplyUpdate() error = %v, want %v", err, wantErr)
	}
	if s.busy {
		t.Fatal("busy stayed set after the failed apply")
	}
	if got := s.GetStatus().State; got != StateReady {
		t.Fatalf("State = %v, want StateReady after a rollback", got)
	}
}

// TestServiceStartupSurfacesWorkerOutcome: the install finishes after the UI
// is gone, so the record the worker leaves behind is the only way its result
// reaches the user.
func TestServiceStartupSurfacesWorkerOutcome(t *testing.T) {
	dir := t.TempDir()
	outcomePath, err := OutcomePath(dir)
	if err != nil {
		t.Fatalf("OutcomePath: %v", err)
	}
	want := Outcome{Version: "1.2.3", Error: "installer left the launcher unchanged", FinishedAt: time.Now()}
	if err := writeOutcome(outcomePath, want); err != nil {
		t.Fatalf("writeOutcome: %v", err)
	}

	s := &Service{dir: dir, notes: mustNotesStore(t, dir), store: mustStore(t, dir), client: mustQuietClient(t), currentVersion: "1.0.0"}
	if err := s.ServiceStartup(context.Background(), application.ServiceOptions{}); err != nil {
		t.Fatalf("ServiceStartup: %v", err)
	}
	if err := s.ServiceShutdown(); err != nil {
		t.Fatalf("ServiceShutdown: %v", err)
	}

	got := s.GetOutcome()
	if got.Version != want.Version || got.OK != want.OK || got.Error != want.Error {
		t.Fatalf("GetOutcome() = %+v, want %+v", got, want)
	}
	if _, statErr := os.Stat(outcomePath); !errors.Is(statErr, fs.ErrNotExist) {
		t.Fatalf("outcome file survived startup and would be reported again: %v", statErr)
	}
}

func TestServiceStartupIgnoresStaleOutcome(t *testing.T) {
	dir := t.TempDir()
	outcomePath, err := OutcomePath(dir)
	if err != nil {
		t.Fatalf("OutcomePath: %v", err)
	}
	old := Outcome{Version: "1.2.3", OK: true, FinishedAt: time.Now().Add(-2 * outcomeMaxAge)}
	if err := writeOutcome(outcomePath, old); err != nil {
		t.Fatalf("writeOutcome: %v", err)
	}

	s := &Service{dir: dir, notes: mustNotesStore(t, dir), store: mustStore(t, dir), client: mustQuietClient(t), currentVersion: "1.0.0"}
	if err := s.ServiceStartup(context.Background(), application.ServiceOptions{}); err != nil {
		t.Fatalf("ServiceStartup: %v", err)
	}
	if err := s.ServiceShutdown(); err != nil {
		t.Fatalf("ServiceShutdown: %v", err)
	}

	if got := s.GetOutcome(); got.Version != "" {
		t.Fatalf("GetOutcome() = %+v, want nothing for a day-old record", got)
	}
}

// TestServiceStartupDropsRecordForTheVersionAlreadyInstalled: once the update
// is applied, currentVersion catches up with the stored one. The leftover
// record otherwise offers the running build to itself and keeps its installer
// in the cache forever.
func TestServiceStartupDropsRecordForTheVersionAlreadyInstalled(t *testing.T) {
	dir := t.TempDir()
	cacheDir, err := CacheDir(dir)
	if err != nil {
		t.Fatalf("CacheDir: %v", err)
	}
	content := []byte("installer that has already been applied")
	readyPath := filepath.Join(cacheDir, "1.2.3", "setup.exe")
	writeTestFile(t, readyPath, content)

	art := Artifact{
		OS: runtime.GOOS, Arch: runtime.GOARCH, Kind: KindInstaller, Name: "setup.exe",
		URL: "https://example.com/setup.exe", Size: int64(len(content)), SHA256: sha256Hex(t, content),
	}
	store := mustStore(t, dir)
	if err := store.Save(stored{AvailableVersion: "1.2.3", Artifact: &art, ReadyPath: readyPath}); err != nil {
		t.Fatalf("seed store: %v", err)
	}

	s := &Service{dir: dir, notes: mustNotesStore(t, dir), store: store, client: mustQuietClient(t), currentVersion: "1.2.3"}
	if err := s.ServiceStartup(context.Background(), application.ServiceOptions{}); err != nil {
		t.Fatalf("ServiceStartup: %v", err)
	}
	if err := s.ServiceShutdown(); err != nil {
		t.Fatalf("ServiceShutdown: %v", err)
	}

	if got := s.GetStatus(); got.State != StateIdle || got.AvailableVersion != "" {
		t.Fatalf("status = %+v, want an idle state with no available version", got)
	}
	v, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if v.AvailableVersion != "" || v.Artifact != nil || v.ReadyPath != "" {
		t.Fatalf("stored state still offers the running version: %+v", v)
	}
	if _, statErr := os.Stat(readyPath); !errors.Is(statErr, fs.ErrNotExist) {
		t.Fatalf("installer for the running version stayed in the cache: %v", statErr)
	}
}

func TestBuildCheckStatusDoesNotRecordAVersionThatIsNotNewer(t *testing.T) {
	s := &Service{currentVersion: "1.2.3"}
	m := validManifest()
	art, err := m.ArtifactFor("windows", "amd64")
	if err != nil {
		t.Fatalf("ArtifactFor: %v", err)
	}

	status, st, err := s.buildCheckStatus(Status{State: StateIdle, CurrentVersion: "1.2.3"}, "", nil, m, art, nil, false, nil)
	if err != nil {
		t.Fatalf("buildCheckStatus() error = %v, want nil", err)
	}
	if st.AvailableVersion != "" || st.Notes != "" {
		t.Fatalf("stored = %+v, want no available version recorded when there is no update", st)
	}
	if got := deriveState(st); got != StateIdle {
		t.Fatalf("deriveState(stored) = %v, want StateIdle: the next start must not offer this build to itself", got)
	}
	if status.AvailableVersion != "" {
		t.Fatalf("status.AvailableVersion = %q, want empty", status.AvailableVersion)
	}
	if st.CheckedAt.IsZero() {
		t.Fatal("CheckedAt is zero: the check still happened")
	}
}

func TestCleanupCacheKeepsTheOutcomeFileAndTheWorkerDir(t *testing.T) {
	dir := t.TempDir()
	cacheDir, err := CacheDir(dir)
	if err != nil {
		t.Fatalf("CacheDir: %v", err)
	}
	outcomePath := filepath.Join(cacheDir, outcomeName)
	writeTestFile(t, outcomePath, []byte("{}"))
	workerCopy := filepath.Join(cacheDir, workerDirName, "typhon-update-1.exe")
	writeTestFile(t, workerCopy, []byte("copy"))
	orphan := filepath.Join(cacheDir, "orphan.txt")
	writeTestFile(t, orphan, []byte("x"))

	s := &Service{dir: dir, notes: mustNotesStore(t, dir), currentVersion: "1.0.0"}
	if err := s.cleanupCache(context.Background(), ""); err != nil {
		t.Fatalf("cleanupCache: %v", err)
	}

	if _, statErr := os.Stat(outcomePath); statErr != nil {
		t.Fatalf("cleanup removed the worker outcome before it could be reported: %v", statErr)
	}
	if _, statErr := os.Stat(workerCopy); statErr != nil {
		t.Fatalf("cleanup removed the worker copy, which may still be a running image: %v", statErr)
	}
	if _, statErr := os.Stat(orphan); !errors.Is(statErr, fs.ErrNotExist) {
		t.Fatalf("orphaned file survived cleanup: %v", statErr)
	}
}

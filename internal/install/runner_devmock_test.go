//go:build devmock && !windows

package install

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestMockRunnerInstallWithDestination(t *testing.T) {
	t.Setenv(devmockInstallSecondsEnv, "0")
	dest := filepath.Join(t.TempDir(), "Foo Game")
	logPath := filepath.Join(t.TempDir(), "installer.log")
	r := newRunner(func() string { return "" })
	spec := runSpec{
		Path:          filepath.Join(t.TempDir(), "download", "FooGame-setup.exe"),
		InstallerPath: filepath.Join(t.TempDir(), "download", "FooGame-setup.exe"),
		Destination:   dest,
		LogPath:       logPath,
	}
	code, err := r.run(context.Background(), spec)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}
	exe := filepath.Join(dest, "FooGame.exe")
	if info, statErr := os.Stat(exe); statErr != nil {
		t.Fatalf("stat exe: %v", statErr)
	} else if info.Size() != devmockExeSize {
		t.Fatalf("exe size = %d, want %d", info.Size(), devmockExeSize)
	}
	if _, statErr := os.Stat(filepath.Join(dest, devmockMarkerName)); statErr != nil {
		t.Fatalf("stat marker: %v", statErr)
	}
	if _, statErr := os.Stat(logPath); statErr != nil {
		t.Fatalf("stat log: %v", statErr)
	}
}

func TestMockRunnerInstallWithoutDestinationUsesGamesPath(t *testing.T) {
	t.Setenv(devmockInstallSecondsEnv, "0")
	gamesRoot := t.TempDir()
	r := newRunner(func() string { return gamesRoot })
	spec := runSpec{
		Path:          filepath.Join(t.TempDir(), "FooGame-setup.exe"),
		InstallerPath: filepath.Join(t.TempDir(), "FooGame-setup.exe"),
	}
	code, err := r.run(context.Background(), spec)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}
	dest := filepath.Join(gamesRoot, "FooGame")
	if _, statErr := os.Stat(filepath.Join(dest, "FooGame.exe")); statErr != nil {
		t.Fatalf("stat exe: %v", statErr)
	}
}

func TestMockRunnerInstallEmptyGamesPathErrors(t *testing.T) {
	r := newRunner(func() string { return "" })
	spec := runSpec{
		Path:          filepath.Join(t.TempDir(), "FooGame-setup.exe"),
		InstallerPath: filepath.Join(t.TempDir(), "FooGame-setup.exe"),
	}
	if _, err := r.run(context.Background(), spec); err == nil {
		t.Fatal("expected error for empty games path")
	}
}

func TestMockRunnerInstallEmptyNameErrors(t *testing.T) {
	r := newRunner(func() string { return t.TempDir() })
	spec := runSpec{
		Path:          filepath.Join(t.TempDir(), "setup.exe"),
		InstallerPath: filepath.Join(t.TempDir(), "setup.exe"),
	}
	if _, err := r.run(context.Background(), spec); err == nil {
		t.Fatal("expected error for empty derived name")
	}
}

func TestMockRunnerUninstallRemovesMarkedDir(t *testing.T) {
	dir := t.TempDir()
	installDir := filepath.Join(dir, "Foo Game")
	if err := os.MkdirAll(installDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(installDir, devmockMarkerName), []byte("installer"), 0o600); err != nil {
		t.Fatal(err)
	}
	r := newRunner(func() string { return "" })
	spec := runSpec{Path: filepath.Join(installDir, "unins000.exe")}
	code, err := r.run(context.Background(), spec)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}
	if _, statErr := os.Stat(installDir); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("install dir still present: %v", statErr)
	}
}

func TestMockRunnerUninstallRefusesUnmarkedDir(t *testing.T) {
	dir := t.TempDir()
	installDir := filepath.Join(dir, "Foo Game")
	if err := os.MkdirAll(installDir, 0o755); err != nil {
		t.Fatal(err)
	}
	r := newRunner(func() string { return "" })
	spec := runSpec{Path: filepath.Join(installDir, "unins000.exe")}
	if _, err := r.run(context.Background(), spec); err == nil {
		t.Fatal("expected error removing unmarked directory")
	}
	if _, statErr := os.Stat(installDir); statErr != nil {
		t.Fatalf("install dir removed: %v", statErr)
	}
}

func TestMockRunnerProductCodeUninstallSucceeds(t *testing.T) {
	r := newRunner(func() string { return "" })
	spec := runSpec{
		Path: filepath.Join(t.TempDir(), "system32", "msiexec.exe"),
		Args: []string{"/x", "{PRODUCT-CODE}", "/qn", "/norestart"},
	}
	code, err := r.run(context.Background(), spec)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}
}

func TestMockRunnerCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r := newRunner(func() string { return "" })
	spec := runSpec{Path: filepath.Join(t.TempDir(), "FooGame-setup.exe")}
	if _, err := r.run(ctx, spec); !errors.Is(err, context.Canceled) {
		t.Fatalf("run error = %v, want context.Canceled", err)
	}
}

func TestMockRunnerInvalidElevateFlagErrors(t *testing.T) {
	t.Setenv(devmockElevateEnv, "maybe")
	r := newRunner(func() string { return "" })
	spec := runSpec{
		Path: filepath.Join(t.TempDir(), "FooGame-setup.exe"), StatePath: filepath.Join(t.TempDir(), "state.json"),
	}
	if _, err := r.run(context.Background(), spec); !errors.Is(err, errDevmockInvalidElevateFlag) {
		t.Fatalf("run error = %v, want errDevmockInvalidElevateFlag", err)
	}
}

func TestMockRunnerInvalidInstallSecondsErrors(t *testing.T) {
	t.Setenv(devmockInstallSecondsEnv, "not-a-number")
	r := newRunner(func() string { return t.TempDir() })
	spec := runSpec{
		Path: filepath.Join(t.TempDir(), "FooGame-setup.exe"), InstallerPath: filepath.Join(t.TempDir(), "FooGame-setup.exe"),
	}
	if _, err := r.run(context.Background(), spec); !errors.Is(err, errDevmockInvalidInstallDelay) {
		t.Fatalf("run error = %v, want errDevmockInvalidInstallDelay", err)
	}
}

func TestMockRunnerRunElevateDisabledTakesDirectRoute(t *testing.T) {
	t.Setenv(devmockElevateEnv, "0")
	t.Setenv(devmockInstallSecondsEnv, "0")
	dest := filepath.Join(t.TempDir(), "Foo Game")
	spec := runSpec{
		Path: filepath.Join(t.TempDir(), "FooGame-setup.exe"), InstallerPath: filepath.Join(t.TempDir(), "FooGame-setup.exe"),
		Destination: dest, StatePath: filepath.Join(t.TempDir(), "state.json"),
	}
	r := newRunner(func() string { return "" })
	code, err := r.run(context.Background(), spec)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}
	if _, statErr := os.Stat(filepath.Join(dest, "FooGame.exe")); statErr != nil {
		t.Fatalf("stat exe: %v, direct install route was not taken", statErr)
	}
}

// fakeWorkerHandle stands in for the real elevated worker process in the
// tests below: runElevated only cares that wait() blocks until told to stop
// and reports a code, so a channel-driven fake keeps these tests fast and
// deterministic instead of racing a real subprocess against the poll timing.
type fakeWorkerHandle struct {
	exit chan struct{}
	code int
	err  error
}

func newFakeWorkerHandle(t *testing.T) *fakeWorkerHandle {
	t.Helper()
	h := &fakeWorkerHandle{exit: make(chan struct{})}
	t.Cleanup(func() { close(h.exit) })
	return h
}

func (h *fakeWorkerHandle) wait() (int, error) {
	<-h.exit
	return h.code, h.err
}

func (*fakeWorkerHandle) close() {}

// TestMockRunnerRunElevateEnabledUsesWorkerProtocol проверяет реальный
// маршрут через runElevated из mockRunner.run: spec-файл действительно
// пишется на диск, а state-файл, который "воркер" пишет асинхронно,
// подхватывается опросом, а не через выход процесса (fakeWorkerHandle висит
// на канале до конца теста).
func TestMockRunnerRunElevateEnabledUsesWorkerProtocol(t *testing.T) {
	t.Setenv(devmockElevateEnv, "1")
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")
	spec := runSpec{
		Path: filepath.Join(dir, "setup.exe"), InstallerPath: filepath.Join(dir, "setup.exe"),
		StatePath: statePath, CancelPath: filepath.Join(dir, "cancel"), ID: "e1",
	}
	withWorkerSeams(t, func(runSpec) (workerHandle, error) {
		return newFakeWorkerHandle(t), nil
	})

	go func() {
		<-time.After(60 * time.Millisecond)
		if err := writeWorkerState(statePath, workerState{Done: true, Code: 0}); err != nil {
			t.Errorf("writeWorkerState: %v", err)
		}
	}()

	r := newRunner(func() string { return "" })
	code, err := r.run(context.Background(), spec)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}

	specFile := workerSpecFilePath(dir, spec.ID)
	written, err := readWorkerSpec(specFile)
	if err != nil {
		t.Fatalf("readWorkerSpec: %v", err)
	}
	if written.InstallerPath != spec.InstallerPath || written.StatePath != statePath {
		t.Fatalf("written worker spec = %+v, does not carry through the mock run spec", written)
	}
}

func TestMockRunnerRunElevateEnabledCancelWritesMarker(t *testing.T) {
	t.Setenv(devmockElevateEnv, "1")
	dir := t.TempDir()
	cancelPath := filepath.Join(dir, "cancel")
	spec := runSpec{
		Path: filepath.Join(dir, "setup.exe"), InstallerPath: filepath.Join(dir, "setup.exe"),
		StatePath: filepath.Join(dir, "state.json"), CancelPath: cancelPath, ID: "e2",
	}
	withWorkerSeams(t, func(runSpec) (workerHandle, error) {
		return newFakeWorkerHandle(t), nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-time.After(30 * time.Millisecond)
		cancel()
	}()

	r := newRunner(func() string { return "" })
	if _, err := r.run(ctx, spec); err == nil {
		t.Fatal("run returned nil error after cancellation")
	}
	if !workerCancelRequested(cancelPath) {
		t.Fatal("cancel marker was not created")
	}
}

func TestMockRunnerDirectInstallCancelledDuringDelayWritesNothing(t *testing.T) {
	t.Setenv(devmockElevateEnv, "0")
	t.Setenv(devmockInstallSecondsEnv, "1")
	dest := filepath.Join(t.TempDir(), "Foo Game")
	spec := runSpec{
		Path: filepath.Join(t.TempDir(), "FooGame-setup.exe"), InstallerPath: filepath.Join(t.TempDir(), "FooGame-setup.exe"),
		Destination: dest,
	}
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-time.After(50 * time.Millisecond)
		cancel()
	}()

	r := newRunner(func() string { return "" })
	start := time.Now()
	if _, err := r.run(ctx, spec); !errors.Is(err, context.Canceled) {
		t.Fatalf("run error = %v, want context.Canceled", err)
	}
	if elapsed := time.Since(start); elapsed > 900*time.Millisecond {
		t.Fatalf("run took %v, want to return promptly on cancellation instead of waiting out the full delay", elapsed)
	}
	if exists(dest) {
		t.Fatal("destination created despite cancellation during the install delay")
	}
}

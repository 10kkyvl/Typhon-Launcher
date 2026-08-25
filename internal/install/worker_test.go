//go:build windows

package install

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"golang.org/x/sys/windows"

	"github.com/wailsapp/wails/v3/pkg/application"
)

func TestWorkerSpecRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "spec.json")
	want := workerSpec{
		ID: "abc123", InstallerPath: `C:\downloads\setup.exe`, Engine: EngineInno,
		Destination: `C:\Games\Foo`, WorkingDir: `C:\downloads`, LogPath: `C:\log\i.log`,
		InfPath: `C:\log\i.inf`, StatePath: `C:\log\i-state.json`, CancelPath: `C:\log\i-cancel`,
		Options: installOptions{SkipShortcuts: true, SkipExtras: true}, Background: true, Hidden: true,
	}
	if err := writeWorkerSpec(path, want); err != nil {
		t.Fatalf("writeWorkerSpec: %v", err)
	}
	got, err := readWorkerSpec(path)
	if err != nil {
		t.Fatalf("readWorkerSpec: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("readWorkerSpec = %+v, want %+v", got, want)
	}
}

func TestReadWorkerSpecMissingFile(t *testing.T) {
	_, err := readWorkerSpec(filepath.Join(t.TempDir(), "missing.json"))
	if err == nil {
		t.Fatal("readWorkerSpec on missing file returned nil error")
	}
}

func TestReadWorkerSpecCorruptJSONKeepsPartialFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "spec.json")
	statePath := filepath.Join(dir, "state.json")
	raw := fmt.Sprintf(`{"statePath": %q, "options": "not-an-object"}`, statePath)
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatalf("write raw spec: %v", err)
	}
	spec, err := readWorkerSpec(path)
	if err == nil {
		t.Fatal("readWorkerSpec on corrupt json returned nil error")
	}
	if spec.StatePath != statePath {
		t.Fatalf("spec.StatePath = %q, want %q (partial decode lost)", spec.StatePath, statePath)
	}
}

func TestWriteWorkerSpecEmptyPath(t *testing.T) {
	if err := writeWorkerSpec("", workerSpec{}); err == nil {
		t.Fatal("writeWorkerSpec(\"\", ...) returned nil error")
	}
}

func TestWorkerStateRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "state.json")
	want := workerState{PID: 4242, Phase: string(workerPhaseInstalling), Code: 3010, Done: true, Components: []string{"a", "b"}}
	if err := writeWorkerState(path, want); err != nil {
		t.Fatalf("writeWorkerState: %v", err)
	}
	got, found, err := readWorkerState(path)
	if err != nil {
		t.Fatalf("readWorkerState: %v", err)
	}
	if !found {
		t.Fatal("readWorkerState found = false, want true")
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("readWorkerState = %+v, want %+v", got, want)
	}
}

func TestReadWorkerStateMissingFileIsEmptyNotError(t *testing.T) {
	state, found, err := readWorkerState(filepath.Join(t.TempDir(), "missing.json"))
	if err != nil {
		t.Fatalf("readWorkerState missing file error = %v, want nil", err)
	}
	if found {
		t.Fatal("readWorkerState found = true for missing file")
	}
	if !reflect.DeepEqual(state, workerState{}) {
		t.Fatalf("readWorkerState state = %+v, want zero value", state)
	}
}

func TestReadWorkerStateCorruptJSONIsError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatalf("write raw state: %v", err)
	}
	_, found, err := readWorkerState(path)
	if err == nil {
		t.Fatal("readWorkerState on corrupt json returned nil error")
	}
	if found {
		t.Fatal("readWorkerState found = true on corrupt json")
	}
}

func TestReadWorkerStateEmptyPath(t *testing.T) {
	_, _, err := readWorkerState("")
	if !errors.Is(err, errWorkerStatePathUnavailable) {
		t.Fatalf("readWorkerState(\"\") error = %v, want errWorkerStatePathUnavailable", err)
	}
}

func TestWriteWorkerStateEmptyPath(t *testing.T) {
	if err := writeWorkerState("", workerState{}); !errors.Is(err, errWorkerStatePathUnavailable) {
		t.Fatalf("writeWorkerState(\"\", ...) error = %v, want errWorkerStatePathUnavailable", err)
	}
}

func TestWorkerCancelMarker(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "cancel")

	if workerCancelRequested(path) {
		t.Fatal("workerCancelRequested = true before marker created")
	}
	if err := writeWorkerCancel(path); err != nil {
		t.Fatalf("writeWorkerCancel: %v", err)
	}
	if !workerCancelRequested(path) {
		t.Fatal("workerCancelRequested = false after marker created")
	}
	if err := clearWorkerCancel(path); err != nil {
		t.Fatalf("clearWorkerCancel: %v", err)
	}
	if workerCancelRequested(path) {
		t.Fatal("workerCancelRequested = true after marker cleared")
	}
	if err := clearWorkerCancel(path); err != nil {
		t.Fatalf("clearWorkerCancel on already-missing marker: %v", err)
	}
}

func TestClearWorkerCancelPropagatesRemoveFailure(t *testing.T) {
	dir := t.TempDir()
	blocked := filepath.Join(dir, "blocked")
	if err := os.Mkdir(blocked, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(blocked, "keep.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write file inside dir: %v", err)
	}
	if err := clearWorkerCancel(blocked); err == nil {
		t.Fatal("clearWorkerCancel on non-empty directory returned nil error")
	}
}

func TestWatchWorkerCancelCancelsContext(t *testing.T) {
	restore := workerCancelPollInterval
	workerCancelPollInterval = 5 * time.Millisecond
	t.Cleanup(func() { workerCancelPollInterval = restore })

	dir := t.TempDir()
	path := filepath.Join(dir, "cancel")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		watchWorkerCancel(ctx, path, cancel)
		close(done)
	}()

	if err := writeWorkerCancel(path); err != nil {
		t.Fatalf("writeWorkerCancel: %v", err)
	}

	select {
	case <-ctx.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("ctx was not cancelled after cancel marker appeared")
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("watchWorkerCancel did not return after cancelling ctx")
	}
}

func TestWatchWorkerCancelReturnsImmediatelyWithoutPath(t *testing.T) {
	done := make(chan struct{})
	go func() {
		watchWorkerCancel(context.Background(), "", func() {})
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("watchWorkerCancel(\"\") did not return")
	}
}

func TestDiscoverComponentsSkipsWhenNotInno(t *testing.T) {
	in := discoverySpec{Engine: EngineNsis, Options: installOptions{SkipExtras: true, SkipShortcuts: true}}
	components, reason, err := discoverComponents(context.Background(), in)
	if err != nil || reason != "" || components != nil {
		t.Fatalf("discoverComponents = (%v, %q, %v), want (nil, \"\", nil)", components, reason, err)
	}
}

func TestDiscoverComponentsSkipsWhenNoOptionsRequested(t *testing.T) {
	in := discoverySpec{Engine: EngineInno}
	components, reason, err := discoverComponents(context.Background(), in)
	if err != nil || reason != "" || components != nil {
		t.Fatalf("discoverComponents = (%v, %q, %v), want (nil, \"\", nil)", components, reason, err)
	}
}

func TestShouldDiscoverComponents(t *testing.T) {
	cases := []struct {
		name string
		in   discoverySpec
		want bool
	}{
		{"inno with skip extras", discoverySpec{Engine: EngineInno, Options: installOptions{SkipExtras: true}}, true},
		{"inno with skip shortcuts", discoverySpec{Engine: EngineInno, Options: installOptions{SkipShortcuts: true}}, true},
		{"inno without options", discoverySpec{Engine: EngineInno}, false},
		{"nsis with both options", discoverySpec{Engine: EngineNsis, Options: installOptions{SkipExtras: true, SkipShortcuts: true}}, false},
		{"msi with both options", discoverySpec{Engine: EngineMsi, Options: installOptions{SkipExtras: true, SkipShortcuts: true}}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := shouldDiscoverComponents(tc.in); got != tc.want {
				t.Fatalf("shouldDiscoverComponents(%+v) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func powershellPath(t *testing.T) string {
	t.Helper()
	root := os.Getenv("SystemRoot")
	if root == "" {
		root = `C:\Windows`
	}
	path := filepath.Join(root, "System32", "WindowsPowerShell", "v1.0", "powershell.exe")
	//nolint:gosec // G703: путь строится из %SystemRoot%, а не из ввода пользователя — фиксированный системный бинарь для тестового стенда
	if _, err := os.Stat(path); err != nil {
		t.Skipf("powershell.exe unavailable: %v", err)
	}
	return path
}

// startAwaitedDiscovery is the shared setup used by the awaitDiscoveryIni
// tests below: it starts a real stand-in "installer" (PowerShell) exactly the
// way attemptDiscovery does, so the tests exercise the actual primitive that
// both the worker and the non-elevated processRunner.run path share.
func startAwaitedDiscovery(t *testing.T, ps, dir string, args []string) (*exec.Cmd, windows.Handle) {
	t.Helper()
	rs := runSpec{Path: ps, Args: args, Dir: dir, Hidden: true, Background: true}
	cmd, group, err := startDiscoveryRun(rs)
	if err != nil {
		t.Fatalf("startDiscoveryRun: %v", err)
	}
	return cmd, group
}

func TestAwaitDiscoveryIniDetectsFile(t *testing.T) {
	ps := powershellPath(t)
	dir := t.TempDir()
	infPath := filepath.Join(dir, "probe.inf")

	restore := discoveryPollInterval
	discoveryPollInterval = 20 * time.Millisecond
	t.Cleanup(func() { discoveryPollInterval = restore })

	cmd, group := startAwaitedDiscovery(t, ps, dir, []string{
		"-NoProfile", "-NonInteractive", "-Command",
		fmt.Sprintf("New-Item -ItemType File -Path '%s' -Force | Out-Null; Start-Sleep -Seconds 30", infPath),
	})

	start := time.Now()
	reason, err := awaitDiscoveryIni(context.Background(), cmd, group, infPath, ps)
	if err != nil {
		t.Fatalf("awaitDiscoveryIni error = %v", err)
	}
	if reason != "" {
		t.Fatalf("reason = %q, want empty", reason)
	}
	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Fatalf("awaitDiscoveryIni took %v, want prompt termination once the ini appears", elapsed)
	}
	if !exists(infPath) {
		t.Fatal("ini file was not created by the discovery pass")
	}
}

func TestAwaitDiscoveryIniTimesOutWhenFileNeverAppears(t *testing.T) {
	ps := powershellPath(t)
	dir := t.TempDir()
	infPath := filepath.Join(dir, "probe.inf")

	restorePoll, restoreTimeout := discoveryPollInterval, discoveryTimeout
	discoveryPollInterval = 20 * time.Millisecond
	discoveryTimeout = 300 * time.Millisecond
	t.Cleanup(func() { discoveryPollInterval, discoveryTimeout = restorePoll, restoreTimeout })

	cmd, group := startAwaitedDiscovery(t, ps, dir, []string{"-NoProfile", "-NonInteractive", "-Command", "Start-Sleep -Seconds 30"})

	start := time.Now()
	reason, err := awaitDiscoveryIni(context.Background(), cmd, group, infPath, ps)
	if err != nil {
		t.Fatalf("awaitDiscoveryIni error = %v", err)
	}
	if reason == "" {
		t.Fatal("reason = empty, want a timeout explanation")
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("awaitDiscoveryIni took %v, want to respect discoveryTimeout", elapsed)
	}
	if exists(infPath) {
		t.Fatal("ini file should not have been created")
	}
}

func TestAwaitDiscoveryIniReportsEarlyExit(t *testing.T) {
	ps := powershellPath(t)
	dir := t.TempDir()
	infPath := filepath.Join(dir, "probe.inf")

	cmd, group := startAwaitedDiscovery(t, ps, dir, []string{"-NoProfile", "-NonInteractive", "-Command", "exit 0"})

	reason, err := awaitDiscoveryIni(context.Background(), cmd, group, infPath, ps)
	if err != nil {
		t.Fatalf("awaitDiscoveryIni error = %v", err)
	}
	if reason == "" {
		t.Fatal("reason = empty, want an early-exit explanation")
	}
}

func TestAwaitDiscoveryIniRespectsCancellation(t *testing.T) {
	ps := powershellPath(t)
	dir := t.TempDir()
	infPath := filepath.Join(dir, "probe.inf")

	cmd, group := startAwaitedDiscovery(t, ps, dir, []string{"-NoProfile", "-NonInteractive", "-Command", "Start-Sleep -Seconds 30"})

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	start := time.Now()
	reason, err := awaitDiscoveryIni(ctx, cmd, group, infPath, ps)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("awaitDiscoveryIni error = %v, want context.DeadlineExceeded", err)
	}
	if reason != "" {
		t.Fatalf("reason = %q, want empty on the error path", reason)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("awaitDiscoveryIni took %v after cancellation", elapsed)
	}
}

func TestAttemptDiscoveryRequestsElevationWhenInstallerNeedsIt(t *testing.T) {
	if windows.GetCurrentProcessToken().IsElevated() {
		t.Skip("тест идёт с правами администратора: ERROR_ELEVATION_REQUIRED не воспроизводится")
	}
	const path = `C:\Windows\regedit.exe`
	if _, err := os.Stat(path); err != nil {
		t.Skipf("нет %s: %v", path, err)
	}
	dir := t.TempDir()
	in := discoverySpec{
		Engine: EngineInno, InstallerPath: path, Destination: filepath.Join(dir, "dest"),
		WorkingDir: dir, InfPath: filepath.Join(dir, "probe.inf"),
		Options: installOptions{SkipShortcuts: true},
	}
	outcome, err := attemptDiscovery(context.Background(), in)
	if err != nil {
		t.Fatalf("attemptDiscovery error = %v", err)
	}
	if !outcome.elevate {
		t.Fatalf("outcome = %+v, want elevate=true", outcome)
	}
	if outcome.components != nil || outcome.reason != "" {
		t.Fatalf("outcome = %+v, want only elevate set", outcome)
	}
}

func TestApplyDiscoveredComponentsAddsComponentsFlag(t *testing.T) {
	spec := runSpec{
		Path: `C:\g\setup.exe`, InstallerPath: `C:\g\setup.exe`, Engine: EngineInno,
		Destination: `C:\Games\Foo`, Args: []string{"placeholder"},
	}
	got, err := applyDiscoveredComponents(spec, []string{"compgame", "lang"})
	if err != nil {
		t.Fatalf("applyDiscoveredComponents error = %v", err)
	}
	want := "/COMPONENTS=compgame,lang"
	found := false
	for _, a := range got.Args {
		if a == want {
			found = true
		}
	}
	if !found {
		t.Fatalf("Args = %v, want to contain %q", got.Args, want)
	}
}

func TestApplyDiscoveredComponentsNoopWithoutComponents(t *testing.T) {
	spec := runSpec{Path: `C:\g\setup.exe`, InstallerPath: `C:\g\setup.exe`, Engine: EngineInno, Destination: `C:\Games\Foo`, Args: []string{"orig"}}
	got, err := applyDiscoveredComponents(spec, nil)
	if err != nil {
		t.Fatalf("applyDiscoveredComponents error = %v", err)
	}
	if !reflect.DeepEqual(got, spec) {
		t.Fatalf("applyDiscoveredComponents modified spec without components: %+v", got)
	}
}

func TestApplyDiscoveredComponentsPropagatesBuildError(t *testing.T) {
	spec := runSpec{Path: `C:\g\setup.exe`, InstallerPath: `C:\g\setup.exe`, Engine: EngineInno}
	if _, err := applyDiscoveredComponents(spec, []string{"compgame"}); err == nil {
		t.Fatal("applyDiscoveredComponents with empty destination returned nil error")
	}
}

// TestProcessRunnerContinuesAfterDiscoveryFailure проверяет саму дыру,
// которую закрывает эта правка: до фикса processRunner.run вообще не звал
// attemptDiscovery на неэлевированном пути, поэтому установщик, не
// требующий UAC, получал /MERGETASKS без /COMPONENTS и ставил вычеркнутые
// компоненты как раньше. Здесь discovery заведомо проваливается (PowerShell
// не понимает настоящие ключи Inno и завершается почти сразу), и тест
// убеждается, что это не останавливает установку — основной прогон всё равно
// выполняется.
func TestProcessRunnerContinuesAfterDiscoveryFailure(t *testing.T) {
	ps := powershellPath(t)
	dir := t.TempDir()

	spec := runSpec{
		Path: ps, InstallerPath: ps, Dir: dir, Hidden: true,
		Engine: EngineInno, Destination: filepath.Join(dir, "dest"),
		InfPath: filepath.Join(dir, "probe.inf"),
		Options: installOptions{SkipShortcuts: true},
		Args:    []string{"-NoProfile", "-NonInteractive", "-Command", "exit 0"},
	}
	// Стоит стейл-файл от прошлой (гипотетической) разведки: attemptDiscovery
	// безусловно удаляет его перед стартом, поэтому его исчезновение —
	// доказательство того, что разведка на неэлевированном пути реально
	// запускалась, а не только то, что основной прогон не упал. До фикса
	// processRunner.run вообще не звал attemptDiscovery, и этот файл остался
	// бы лежать нетронутым.
	mkFile(t, spec.InfPath, 4)

	code, err := (processRunner{}).run(context.Background(), spec)
	if err != nil {
		t.Fatalf("processRunner.run error = %v, a failed discovery must not fail the install", err)
	}
	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}
	if exists(spec.InfPath) {
		t.Fatal("stale ini file survived: attemptDiscovery was not actually invoked on the non-elevated path")
	}
}

func TestFinishElevatedState(t *testing.T) {
	if code, err := finishElevatedState(workerState{Code: 5}); err != nil || code != 5 {
		t.Fatalf("finishElevatedState = (%d, %v), want (5, nil)", code, err)
	}
	code, err := finishElevatedState(workerState{Code: 5, Error: "boom"})
	if code != 5 || err == nil || err.Error() != "boom" {
		t.Fatalf("finishElevatedState = (%d, %v), want (5, \"boom\")", code, err)
	}
}

// TestFinishElevatedStateCancelled ловит потерю опознаваемости отмены: до
// фикса finishElevatedState заворачивал любую ошибку через errors.New(text),
// и errors.Is(err, context.Canceled) для отменённой установки был ложным —
// Service.fail (service.go) не отличил бы отмену от обычного провала, если
// бы дошёл до этой ветки не через j.cancelled/s.closing.
func TestFinishElevatedStateCancelled(t *testing.T) {
	_, err := finishElevatedState(workerState{Code: 0, Error: context.Canceled.Error(), Cancelled: true})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("finishElevatedState(Cancelled=true) = %v, want errors.Is(err, context.Canceled)", err)
	}
}

func TestReadFinalWorkerStateNotFound(t *testing.T) {
	_, err := readFinalWorkerState(filepath.Join(t.TempDir(), "missing.json"))
	if !errors.Is(err, errWorkerNotFinished) {
		t.Fatalf("readFinalWorkerState = %v, want errWorkerNotFinished", err)
	}
}

func TestReadFinalWorkerStateNotDone(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	if err := writeWorkerState(path, workerState{Done: false}); err != nil {
		t.Fatalf("writeWorkerState: %v", err)
	}
	_, err := readFinalWorkerState(path)
	if !errors.Is(err, errWorkerNotFinished) {
		t.Fatalf("readFinalWorkerState = %v, want errWorkerNotFinished", err)
	}
}

func TestReadFinalWorkerStateCorruptDoesNotMasqueradeAsNotFinished(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatalf("write raw state: %v", err)
	}
	_, err := readFinalWorkerState(path)
	if err == nil {
		t.Fatal("readFinalWorkerState on corrupt json returned nil error")
	}
	if errors.Is(err, errWorkerNotFinished) {
		t.Fatalf("readFinalWorkerState = %v, must not collapse a real read error into errWorkerNotFinished", err)
	}
}

// testCmdProcess launches a short cmd.exe as a stand-in for the elevated
// worker process, without requiring UAC: runElevated only needs a real
// waitable process handle, the target process itself never has to behave like
// the launcher. The handle is opened independently by pid so closing it
// inside elevatedProc cannot race the *exec.Cmd bookkeeping.
func testCmdProcess(t *testing.T, args ...string) *elevatedProc {
	t.Helper()
	cmdExe := filepath.Join(os.Getenv("SystemRoot"), "System32", "cmd.exe")
	//nolint:gosec // G204: cmd.exe — фиксированный системный бинарь для тестового стенда, args заданы вызывающим тестом, не пользователем
	cmd := exec.Command(cmdExe, args...)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start stand-in process: %v", err)
	}
	//nolint:gosec // G115: pid, выданный ядром, в uint32 помещается всегда
	handle, err := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_QUERY_INFORMATION, false, uint32(cmd.Process.Pid))
	if err != nil {
		t.Fatalf("OpenProcess: %v", err)
	}
	t.Cleanup(func() {
		if err := cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
			t.Logf("kill stand-in process: %v", err)
		}
		if err := cmd.Wait(); err != nil {
			t.Logf("wait stand-in process: %v", err)
		}
	})
	return &elevatedProc{handle: handle}
}

func withWorkerSeams(t *testing.T, launcher func(runSpec) (*elevatedProc, error)) {
	t.Helper()
	restoreLauncher := startElevatedWorker
	restorePoll := workerPollInterval
	restoreWait := workerCancelWait
	startElevatedWorker = launcher
	workerPollInterval = 20 * time.Millisecond
	workerCancelWait = 300 * time.Millisecond
	t.Cleanup(func() {
		startElevatedWorker = restoreLauncher
		workerPollInterval = restorePoll
		workerCancelWait = restoreWait
	})
}

func TestRunElevatedReturnsErrorWhenStateStaysUnfinished(t *testing.T) {
	dir := t.TempDir()
	spec := runSpec{
		Path: `C:\fake\installer.exe`, ID: "t1",
		StatePath: filepath.Join(dir, "state.json"), CancelPath: filepath.Join(dir, "cancel"),
	}
	withWorkerSeams(t, func(runSpec) (*elevatedProc, error) {
		return testCmdProcess(t, "/C", "exit", "0"), nil
	})

	code, err := runElevated(context.Background(), spec)
	if !errors.Is(err, errWorkerNotFinished) {
		t.Fatalf("runElevated error = %v, want errWorkerNotFinished", err)
	}
	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}

	specFile := workerSpecFilePath(dir, spec.ID)
	written, err := readWorkerSpec(specFile)
	if err != nil {
		t.Fatalf("readWorkerSpec: %v", err)
	}
	if written.StatePath != spec.StatePath || written.CancelPath != spec.CancelPath {
		t.Fatalf("written worker spec = %+v, does not carry through StatePath/CancelPath", written)
	}
}

func TestRunElevatedPicksUpStateThroughPolling(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")
	spec := runSpec{Path: `C:\fake\installer.exe`, ID: "t2", StatePath: statePath, CancelPath: filepath.Join(dir, "cancel")}

	// Ping keeps the stand-in process alive well past the poll interval, so a
	// success here proves the state-file poll fired, not the process-exit path.
	withWorkerSeams(t, func(runSpec) (*elevatedProc, error) {
		return testCmdProcess(t, "/C", "ping", "-n", "10", "127.0.0.1", ">NUL"), nil
	})

	go func() {
		<-time.After(60 * time.Millisecond)
		if err := writeWorkerState(statePath, workerState{Done: true, Code: 42}); err != nil {
			t.Errorf("writeWorkerState: %v", err)
		}
	}()

	start := time.Now()
	code, err := runElevated(context.Background(), spec)
	if err != nil {
		t.Fatalf("runElevated error = %v", err)
	}
	if code != 42 {
		t.Fatalf("code = %d, want 42", code)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("runElevated took %v, want to return once state was polled as done", elapsed)
	}
}

func TestRunElevatedPropagatesWorkerError(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")
	spec := runSpec{Path: `C:\fake\installer.exe`, ID: "t3", StatePath: statePath, CancelPath: filepath.Join(dir, "cancel")}

	withWorkerSeams(t, func(runSpec) (*elevatedProc, error) {
		return testCmdProcess(t, "/C", "ping", "-n", "10", "127.0.0.1", ">NUL"), nil
	})

	go func() {
		<-time.After(60 * time.Millisecond)
		if err := writeWorkerState(statePath, workerState{Done: true, Code: 7, Error: "установщик упал"}); err != nil {
			t.Errorf("writeWorkerState: %v", err)
		}
	}()

	code, err := runElevated(context.Background(), spec)
	if err == nil || err.Error() != "установщик упал" {
		t.Fatalf("runElevated error = %v, want \"установщик упал\"", err)
	}
	if code != 7 {
		t.Fatalf("code = %d, want 7", code)
	}
}

func TestRunElevatedCancellationWritesMarkerAndWaitsForWorker(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")
	cancelPath := filepath.Join(dir, "cancel")
	spec := runSpec{Path: `C:\fake\installer.exe`, ID: "t4", StatePath: statePath, CancelPath: cancelPath}

	withWorkerSeams(t, func(runSpec) (*elevatedProc, error) {
		return testCmdProcess(t, "/C", "ping", "-n", "30", "127.0.0.1", ">NUL"), nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-time.After(60 * time.Millisecond)
		cancel()
	}()

	// Simulates the real worker reacting to the cancel marker: notices it,
	// then finalizes state, exactly as watchWorkerCancel + RunWorker would.
	go func() {
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			if workerCancelRequested(cancelPath) {
				if err := writeWorkerState(statePath, workerState{Done: true, Error: context.Canceled.Error(), Cancelled: true}); err != nil {
					t.Errorf("writeWorkerState: %v", err)
				}
				return
			}
			<-time.After(10 * time.Millisecond)
		}
		t.Error("cancel marker never appeared")
	}()

	start := time.Now()
	_, err := runElevated(ctx, spec)
	if err == nil {
		t.Fatal("runElevated returned nil error after cancellation")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("runElevated error = %v, want errors.Is(err, context.Canceled)", err)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("runElevated took %v, want to return promptly once the worker reacted", elapsed)
	}
	if !workerCancelRequested(cancelPath) {
		t.Fatal("cancel marker was not created")
	}
}

func TestRunElevatedCancellationTimesOutWhenWorkerNeverResponds(t *testing.T) {
	dir := t.TempDir()
	spec := runSpec{
		Path: `C:\fake\installer.exe`, ID: "t5",
		StatePath: filepath.Join(dir, "state.json"), CancelPath: filepath.Join(dir, "cancel"),
	}

	withWorkerSeams(t, func(runSpec) (*elevatedProc, error) {
		return testCmdProcess(t, "/C", "ping", "-n", "30", "127.0.0.1", ">NUL"), nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-time.After(30 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	_, err := runElevated(ctx, spec)
	if err == nil {
		t.Fatal("runElevated returned nil error though the worker never responded")
	}
	elapsed := time.Since(start)
	if elapsed < workerCancelWait {
		t.Fatalf("runElevated returned after %v, before the cancel wait deadline of %v", elapsed, workerCancelWait)
	}
	if elapsed > workerCancelWait+3*time.Second {
		t.Fatalf("runElevated took %v, far past the cancel wait deadline of %v", elapsed, workerCancelWait)
	}
}

func TestRunWorkerRecordsStateWhenSpecStatePathIsKnown(t *testing.T) {
	dir := t.TempDir()
	specPath := filepath.Join(dir, "spec.json")
	statePath := filepath.Join(dir, "state.json")
	raw := fmt.Sprintf(`{"statePath": %q, "options": "not-an-object"}`, statePath)
	if err := os.WriteFile(specPath, []byte(raw), 0o644); err != nil {
		t.Fatalf("write raw spec: %v", err)
	}

	if err := RunWorker(specPath); err == nil {
		t.Fatal("RunWorker on a broken spec returned nil error")
	}

	state, found, err := readWorkerState(statePath)
	if err != nil {
		t.Fatalf("readWorkerState: %v", err)
	}
	if !found || !state.Done || state.Error == "" {
		t.Fatalf("state = %+v, want a done state carrying the spec error", state)
	}
}

func TestRunWorkerReturnsErrorWhenSpecUnreadable(t *testing.T) {
	if err := RunWorker(filepath.Join(t.TempDir(), "missing.json")); err == nil {
		t.Fatal("RunWorker on a missing spec file returned nil error")
	}
}

// TestDiscardSilentKeepsDirectoryWhenInstallerNotConfirmedStopped закрывает
// КРИТ-1: errInstallerNotConfirmedStopped значит, что процесс, возможно, всё
// ещё пишет в Destination, и RemoveAll на него — гонка на единственной копии
// данных (инвариант 9).
func TestDiscardSilentKeepsDirectoryWhenInstallerNotConfirmedStopped(t *testing.T) {
	s, _, _ := newTestService(t)
	dest := filepath.Join(t.TempDir(), "Game")
	mkFile(t, filepath.Join(dest, "file.bin"), 16)

	cause := fmt.Errorf("%w: %w", errInstallerNotConfirmedStopped, context.Canceled)
	s.discardSilent(Installation{Destination: dest}, fsSnapshot{}, cause)

	if !exists(dest) {
		t.Fatal("destination removed even though the installer never confirmed it stopped")
	}
}

func TestDiscardSilentRemovesDirectoryForOrdinaryFailure(t *testing.T) {
	s, _, _ := newTestService(t)
	dest := filepath.Join(t.TempDir(), "Game")
	mkFile(t, filepath.Join(dest, "file.bin"), 16)

	s.discardSilent(Installation{Destination: dest}, fsSnapshot{}, errors.New("boom"))

	if exists(dest) {
		t.Fatal("destination should have been removed for an ordinary failure")
	}
}

// TestFailKeepsInstallerStillRunningAsFailedNotCancelled проверяет, что
// errInstallerNotConfirmedStopped переживает даже j.cancelled=true: до фикса
// Service.fail сначала смотрел j.cancelled и помечал запись чисто отменённой
// ("ничего не осталось"), хотя установщик мог остаться жив и писать дальше.
func TestFailKeepsInstallerStillRunningAsFailedNotCancelled(t *testing.T) {
	s, _, _ := newTestService(t)
	const id = "f1"
	s.mu.Lock()
	s.items = append(s.items, &Installation{ID: id, Name: "Game", Status: StatusInstalling})
	s.jobs[id] = &job{cancel: func() {}, cancelled: true}
	s.mu.Unlock()

	cause := fmt.Errorf("%w: %w", errInstallerNotConfirmedStopped, context.Canceled)
	s.fail(id, cause)

	got, ok := s.snapshot(id)
	if !ok {
		t.Fatal("record disappeared")
	}
	if got.Status != StatusFailed {
		t.Fatalf("status = %s, want %s even though the job was marked cancelled", got.Status, StatusFailed)
	}
	if got.Error == "" {
		t.Fatal("failed record must carry an error")
	}
}

// TestServiceStartupResumesLiveWorkerAndFinalizes закрывает КРИТ-2: ловит
// самовоспроизводимую версию того, что произошло у пользователя — лаунчер
// перезапускается посреди тихой установки, воркер продолжает жить и
// дописывает игру, а запись должна не потеряться в StatusInterrupted, а
// дождаться Done и зарегистрироваться с честно неизвестным происхождением.
func TestServiceStartupResumesLiveWorkerAndFinalizes(t *testing.T) {
	dir := t.TempDir()
	s := mustServiceAt(t, dir)
	s.settings = newTestSettings(t)
	downloads := newFakeDownloads()
	registrar := &fakeRegistrar{}
	s.downloads = downloads
	s.library = registrar

	dest := filepath.Join(t.TempDir(), "Game")
	exe := filepath.Join(dest, "Game.exe")
	mkFile(t, exe, 4096)

	const id = "resume1"
	item := Installation{
		ID: id, DownloadID: "d1", Name: "Game", Type: TypeExeInstaller,
		Status: StatusInstalling, Destination: dest, Executable: exe,
		Engine: EngineInno, Silent: true, StartedAt: time.Now(),
	}
	if err := s.store.save([]Installation{item}); err != nil {
		t.Fatalf("seed store: %v", err)
	}

	statePath := s.workerStatePath(id)
	if err := writeWorkerState(statePath, workerState{PID: os.Getpid(), Phase: string(workerPhaseInstalling)}); err != nil {
		t.Fatalf("write worker state: %v", err)
	}

	restore := resumeWatchPollInterval
	resumeWatchPollInterval = 20 * time.Millisecond
	t.Cleanup(func() { resumeWatchPollInterval = restore })

	if err := s.ServiceStartup(context.Background(), application.ServiceOptions{}); err != nil {
		t.Fatalf("startup: %v", err)
	}
	t.Cleanup(func() {
		if err := s.ServiceShutdown(); err != nil {
			t.Errorf("shutdown: %v", err)
		}
	})

	got, ok := s.snapshot(id)
	if !ok || got.Status != StatusInstalling {
		t.Fatalf("status after startup = %+v, want still installing: the worker (this test process) is alive", got)
	}

	if err := writeWorkerState(statePath, workerState{PID: os.Getpid(), Done: true, Code: 0}); err != nil {
		t.Fatalf("write final worker state: %v", err)
	}

	done := s.waitStatus(t, id, StatusCompleted)
	if done.Owned {
		t.Fatal("Owned = true, want false: origin of a resumed install is not known")
	}
	if !done.UninstallUnknown {
		t.Fatal("UninstallUnknown = false, want true: uninstall record for a resumed install is not known")
	}

	regs := registrar.registered()
	if len(regs) != 1 {
		t.Fatalf("registered games = %d, want 1", len(regs))
	}
	if regs[0].Owned || !regs[0].UninstallUnknown {
		t.Fatalf("registered = %+v, want Owned=false, UninstallUnknown=true", regs[0])
	}
}

// TestRetryRejectedWhileWorkerStillAlive закрывает п.3: статус мог стать
// retryable по устаревшему прочтению, и Retry не должен поднимать второго
// воркера поверх того, что уже пишет по тем же детерминированным путям
// state/spec/cancel.
func TestRetryRejectedWhileWorkerStillAlive(t *testing.T) {
	s, _, _ := newTestService(t)
	const id = "retry1"
	s.mu.Lock()
	s.items = append(s.items, &Installation{ID: id, DownloadID: "d1", Name: "Game", Type: TypeExeInstaller, Status: StatusFailed, Error: "boom"})
	s.mu.Unlock()

	statePath := s.workerStatePath(id)
	if err := writeWorkerState(statePath, workerState{PID: os.Getpid(), Done: false}); err != nil {
		t.Fatalf("write worker state: %v", err)
	}

	if err := s.Retry(id); !errors.Is(err, errInstallerStillRunning) {
		t.Fatalf("Retry error = %v, want errInstallerStillRunning", err)
	}
}

func TestRetryProceedsWhenWorkerStateIsDone(t *testing.T) {
	s, downloads, _ := newTestService(t)
	root := t.TempDir()
	mkInstaller(t, filepath.Join(root, "setup.exe"), "Inno Setup Setup Data (5.6.2) (u)")
	downloads.add("d1", "Game", root)

	const id = "retry2"
	s.mu.Lock()
	s.items = append(s.items, &Installation{ID: id, DownloadID: "d1", Name: "Game", Type: TypeExeInstaller, Status: StatusFailed, Error: "boom"})
	s.mu.Unlock()

	statePath := s.workerStatePath(id)
	if err := writeWorkerState(statePath, workerState{PID: os.Getpid(), Done: true, Code: 0}); err != nil {
		t.Fatalf("write worker state: %v", err)
	}

	s.runner = &fakeRunner{act: func(runSpec) {}}
	if err := s.Retry(id); err != nil {
		t.Fatalf("Retry error = %v, want nil once the worker state is done", err)
	}
}

// TestLimitJobSucceedsRegardlessOfKillOnCloseSupport закрывает п.4: на этой
// машине SetInformationJobObject отклоняет JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
// с ERROR_INVALID_PARAMETER (похоже на вмешательство защитного ПО) — limitJob
// обязан деградировать до приоритета/класса планирования, а не проваливать
// запуск установщика целиком. До фикса (см. ТЕСТЫ в отчёте) ровно эта же
// ошибка ловилась в groupProcess через TestAwaitDiscoveryIni*.
func TestLimitJobSucceedsRegardlessOfKillOnCloseSupport(t *testing.T) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		t.Fatalf("CreateJobObject: %v", err)
	}
	defer func() {
		if err := windows.CloseHandle(job); err != nil {
			t.Errorf("CloseHandle: %v", err)
		}
	}()
	if err := limitJob(job); err != nil {
		t.Fatalf("limitJob: %v", err)
	}
}

// TestCancelResumedInstallWritesMarkerAndWaitsForWorker закрывает последний
// хвост: Cancel() над записью, подхваченной после перезапуска (job для неё
// не заведён), не должен врать "отменено" над установщиком, который
// продолжает писать. Маркер создаётся, статус остаётся рабочим, и только
// когда воркер сам подтверждает Cancelled в состоянии, запись становится
// StatusCancelled через finishResumed -> cancelResumed.
func TestCancelResumedInstallWritesMarkerAndWaitsForWorker(t *testing.T) {
	dir := t.TempDir()
	s := mustServiceAt(t, dir)
	s.settings = newTestSettings(t)
	s.downloads = newFakeDownloads()
	s.library = &fakeRegistrar{}

	dest := filepath.Join(t.TempDir(), "Game")
	const id = "cancel1"
	item := Installation{
		ID: id, DownloadID: "d1", Name: "Game", Type: TypeExeInstaller,
		Status: StatusInstalling, Destination: dest, Engine: EngineInno, Silent: true,
		StartedAt: time.Now(),
	}
	if err := s.store.save([]Installation{item}); err != nil {
		t.Fatalf("seed store: %v", err)
	}

	statePath := s.workerStatePath(id)
	if err := writeWorkerState(statePath, workerState{PID: os.Getpid(), Phase: string(workerPhaseInstalling)}); err != nil {
		t.Fatalf("write worker state: %v", err)
	}

	restore := resumeWatchPollInterval
	resumeWatchPollInterval = 20 * time.Millisecond
	t.Cleanup(func() { resumeWatchPollInterval = restore })

	if err := s.ServiceStartup(context.Background(), application.ServiceOptions{}); err != nil {
		t.Fatalf("startup: %v", err)
	}
	t.Cleanup(func() {
		if err := s.ServiceShutdown(); err != nil {
			t.Errorf("shutdown: %v", err)
		}
	})

	got, ok := s.snapshot(id)
	if !ok || got.Status != StatusInstalling {
		t.Fatalf("status after startup = %+v, want still installing", got)
	}

	if err := s.Cancel(id); err != nil {
		t.Fatalf("Cancel error = %v", err)
	}

	cancelPath := s.workerCancelPath(id)
	if !workerCancelRequested(cancelPath) {
		t.Fatal("cancel marker was not created")
	}

	got, ok = s.snapshot(id)
	if !ok || got.Status != StatusInstalling {
		t.Fatalf("status right after Cancel = %+v, want still installing (not cancelled yet)", got)
	}

	if err := writeWorkerState(statePath, workerState{PID: os.Getpid(), Done: true, Cancelled: true, Error: context.Canceled.Error()}); err != nil {
		t.Fatalf("write final worker state: %v", err)
	}

	done := s.waitStatus(t, id, StatusCancelled)
	if done.Error != "" {
		t.Fatalf("cancelled record carries error = %q, want empty", done.Error)
	}
}

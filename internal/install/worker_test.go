//go:build windows

package install

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

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

// testCmdProcess launches a short cmd.exe as a stand-in for the elevated
// worker process, without requiring UAC: runElevated only needs a real
// waitable process handle, the target process itself never has to behave like
// the launcher. The handle is opened independently by pid so closing it
// inside elevatedProc cannot race the *exec.Cmd bookkeeping.
func testCmdProcess(t *testing.T, args ...string) workerHandle {
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

// TestElevatedProcWaitReturnsExitCode закрывает единственный участок кода,
// который переезд TestRunElevated* на портируемый workerHandle оставил без
// покрытия на Windows: сам awaitProcess (WaitForSingleObject +
// GetExitCodeProcess) через реальный хэндл процесса, а не через выдуманный
// exec.Cmd.Wait() портируемого тестового двойника.
func TestElevatedProcWaitReturnsExitCode(t *testing.T) {
	proc := testCmdProcess(t, "/C", "exit", "3")
	code, err := proc.wait()
	if err != nil {
		t.Fatalf("wait: %v", err)
	}
	if code != 3 {
		t.Fatalf("code = %d, want 3", code)
	}
	proc.close()
}

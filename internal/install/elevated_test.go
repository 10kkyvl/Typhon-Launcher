package install

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"
	"time"
)

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

// Файл состояния один на всю цепочку установщиков, поэтому оставшийся от
// предыдущего прогона Done: true обязан быть отвергнут: без токена прогона
// лаунчер прочитал бы его как результат следующей установки — тем более через
// брокера, который перезаписывает файл заметно позже старта опроса.
func TestReadFinalWorkerStateRejectsThePreviousRun(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	if err := writeWorkerState(path, workerState{Run: "run-1", Done: true, Code: 0}); err != nil {
		t.Fatalf("writeWorkerState: %v", err)
	}
	if _, err := readFinalWorkerState(path, "run-2"); !errors.Is(err, errWorkerNotFinished) {
		t.Fatalf("readFinalWorkerState = %v, ожидался errWorkerNotFinished для чужого прогона", err)
	}
	if _, err := readFinalWorkerState(path, "run-1"); err != nil {
		t.Fatalf("readFinalWorkerState для своего прогона = %v", err)
	}
}

func TestRunElevatedIgnoresTheStateOfThePreviousRun(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")
	if err := writeWorkerState(statePath, workerState{Run: "stale", Done: true, Code: 7}); err != nil {
		t.Fatalf("seed stale state: %v", err)
	}
	spec := runSpec{Path: `C:\fake\installer.exe`, ID: "t10", StatePath: statePath, CancelPath: filepath.Join(dir, "cancel")}

	withWorkerSeams(t, func(launch runSpec) (workerHandle, error) {
		if len(launch.Args) < 2 {
			t.Fatalf("воркер запущен без пути спеки: %v", launch.Args)
		}
		ws, err := readWorkerSpec(launch.Args[1])
		if err != nil {
			return nil, err
		}
		go func() {
			<-time.After(80 * time.Millisecond)
			if err := writeWorkerState(statePath, workerState{Run: ws.Run, Done: true, Code: 3}); err != nil {
				t.Errorf("writeWorkerState: %v", err)
			}
		}()
		return longRunningProcess(t, 10), nil
	})

	code, err := runElevated(context.Background(), spec)
	if err != nil {
		t.Fatalf("runElevated: %v", err)
	}
	if code != 3 {
		t.Fatalf("code = %d, ожидался 3: прочитан результат предыдущего прогона", code)
	}
}

func TestReadFinalWorkerStateNotFound(t *testing.T) {
	_, err := readFinalWorkerState(filepath.Join(t.TempDir(), "missing.json"), "")
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
	_, err := readFinalWorkerState(path, "")
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
	_, err := readFinalWorkerState(path, "")
	if err == nil {
		t.Fatal("readFinalWorkerState on corrupt json returned nil error")
	}
	if errors.Is(err, errWorkerNotFinished) {
		t.Fatalf("readFinalWorkerState = %v, must not collapse a real read error into errWorkerNotFinished", err)
	}
}

// testProcHandle adapts a plain *exec.Cmd to workerHandle so the
// TestRunElevated* tests below can exercise the real runElevated loop against
// an actual waitable process without any OS-specific process API: runElevated
// only needs something that blocks until exit and reports a code, the
// stand-in process itself never has to behave like the launcher.
type testProcHandle struct {
	cmd *exec.Cmd
}

func (h *testProcHandle) wait() (int, error) {
	err := h.cmd.Wait()
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return exit.ExitCode(), nil
	}
	if err != nil {
		return 0, err
	}
	return 0, nil
}

func (*testProcHandle) close() {}

func startStandInProcess(t *testing.T, name string, args ...string) workerHandle {
	t.Helper()
	//nolint:gosec // G204: тестовый стенд-ин процесс, путь и аргументы фиксированы вызывающим тестом, не пользователем
	cmd := exec.Command(name, args...)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start stand-in process: %v", err)
	}
	t.Cleanup(func() {
		if err := cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
			t.Logf("kill stand-in process: %v", err)
		}
	})
	return &testProcHandle{cmd: cmd}
}

// quickExitProcess starts a process that exits almost immediately with code 0.
func quickExitProcess(t *testing.T) workerHandle {
	t.Helper()
	if runtime.GOOS == "windows" {
		cmdExe := filepath.Join(os.Getenv("SystemRoot"), "System32", "cmd.exe")
		return startStandInProcess(t, cmdExe, "/C", "exit", "0")
	}
	return startStandInProcess(t, "/bin/sh", "-c", "exit 0")
}

// longRunningProcess starts a process that stays alive for roughly the given
// number of seconds, long past any poll interval used by the tests below.
func longRunningProcess(t *testing.T, seconds int) workerHandle {
	t.Helper()
	if runtime.GOOS == "windows" {
		cmdExe := filepath.Join(os.Getenv("SystemRoot"), "System32", "cmd.exe")
		return startStandInProcess(t, cmdExe, "/C", "ping", "-n", strconv.Itoa(seconds), "127.0.0.1", ">NUL")
	}
	return startStandInProcess(t, "/bin/sleep", strconv.Itoa(seconds))
}

func withWorkerSeams(t *testing.T, launcher func(runSpec) (workerHandle, error)) {
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

// echoRun повторяет то, что делает настоящий воркер: состояние помечается
// токеном прогона из полученной спеки. Без него подделанное состояние
// принадлежит чужому прогону и по контракту игнорируется.
func echoRun(t *testing.T, dir, id string, state workerState) workerState {
	t.Helper()
	ws, err := readWorkerSpec(workerSpecFilePath(dir, id))
	if err != nil {
		t.Errorf("readWorkerSpec: %v", err)
		return state
	}
	state.Run = ws.Run
	return state
}

func TestRunElevatedReturnsErrorWhenStateStaysUnfinished(t *testing.T) {
	dir := t.TempDir()
	spec := runSpec{
		Path: `C:\fake\installer.exe`, ID: "t1",
		StatePath: filepath.Join(dir, "state.json"), CancelPath: filepath.Join(dir, "cancel"),
	}
	withWorkerSeams(t, func(runSpec) (workerHandle, error) {
		return quickExitProcess(t), nil
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

	// Keeps the stand-in process alive well past the poll interval, so a
	// success here proves the state-file poll fired, not the process-exit path.
	withWorkerSeams(t, func(runSpec) (workerHandle, error) {
		return longRunningProcess(t, 10), nil
	})

	go func() {
		<-time.After(60 * time.Millisecond)
		if err := writeWorkerState(statePath, echoRun(t, dir, spec.ID, workerState{Done: true, Code: 42})); err != nil {
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

	withWorkerSeams(t, func(runSpec) (workerHandle, error) {
		return longRunningProcess(t, 10), nil
	})

	go func() {
		<-time.After(60 * time.Millisecond)
		if err := writeWorkerState(statePath, echoRun(t, dir, spec.ID, workerState{Done: true, Code: 7, Error: "установщик упал"})); err != nil {
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

	withWorkerSeams(t, func(runSpec) (workerHandle, error) {
		return longRunningProcess(t, 30), nil
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
				if err := writeWorkerState(statePath, echoRun(t, dir, spec.ID, workerState{Done: true, Error: context.Canceled.Error(), Cancelled: true})); err != nil {
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

	withWorkerSeams(t, func(runSpec) (workerHandle, error) {
		return longRunningProcess(t, 30), nil
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

// TestRunElevatedSurvivesTransientStateReadFailure закрывает виндовое падение
// CI: воркер подменяет state.json переименованием, и чтение ровно в этот
// момент получает от Windows «файл занят другим процессом». Цикл опроса
// считал любую ошибку чтения провалом установки, так что мгновенная блокировка
// роняла установку, которая на самом деле шла нормально. Каталог на месте
// файла даёт ту же ошибку чтения на любой ОС.
func TestRunElevatedSurvivesTransientStateReadFailure(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")
	if err := os.Mkdir(statePath, 0o755); err != nil {
		t.Fatal(err)
	}
	spec := runSpec{Path: `C:\fake\installer.exe`, ID: "t9", StatePath: statePath, CancelPath: filepath.Join(dir, "cancel")}

	withWorkerSeams(t, func(runSpec) (workerHandle, error) {
		return longRunningProcess(t, 10), nil
	})

	go func() {
		<-time.After(60 * time.Millisecond)
		// Снятие блокировки повторяется: цикл опроса читает этот же путь, и
		// Windows отказывает в удалении, пока держит хэндл от неудавшегося
		// чтения. Одна попытка здесь роняла тест не по существу проверки —
		// сама блокировка снимается, просто не с первого раза.
		if err := removeWithRetry(statePath); err != nil {
			t.Errorf("remove blocking directory: %v", err)
			return
		}
		if err := writeWorkerState(statePath, echoRun(t, dir, spec.ID, workerState{Done: true, Code: 3})); err != nil {
			t.Errorf("writeWorkerState: %v", err)
		}
	}()

	code, err := runElevated(context.Background(), spec)
	if err != nil {
		t.Fatalf("runElevated error = %v, want the transient read to be retried", err)
	}
	if code != 3 {
		t.Fatalf("code = %d, want 3", code)
	}
}

// TestRunElevatedReportsPersistentStateReadFailure — вторая половина: ошибка,
// которая не проходит, остаётся ошибкой, а не полирует установку молчанием.
func TestRunElevatedReportsPersistentStateReadFailure(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")
	if err := os.Mkdir(statePath, 0o755); err != nil {
		t.Fatal(err)
	}
	spec := runSpec{Path: `C:\fake\installer.exe`, ID: "t10", StatePath: statePath, CancelPath: filepath.Join(dir, "cancel")}

	withWorkerSeams(t, func(runSpec) (workerHandle, error) {
		return longRunningProcess(t, 10), nil
	})

	if _, err := runElevated(context.Background(), spec); err == nil {
		t.Fatal("runElevated returned nil for a state file that never became readable")
	}
}

// removeWithRetry снимает блокирующий каталог, не сдаваясь на первой ошибке:
// на Windows удаление конкурирует с читателем того же пути.
func removeWithRetry(path string) error {
	var err error
	for range 100 {
		if err = os.Remove(path); err == nil {
			return nil
		}
		<-time.After(10 * time.Millisecond)
	}
	return err
}

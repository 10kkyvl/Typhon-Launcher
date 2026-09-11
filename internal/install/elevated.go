package install

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"typhon/internal/uierr"
)

const installWorkerFlag = "--install-worker"

var (
	errWorkerStatePath   = errors.New("путь состояния установки не задан")
	errWorkerNotFinished = uierr.New("install.worker_not_finished", "повышенный воркер установки не подтвердил завершение")

	// Подменяются в тестах, чтобы не поднимать настоящий UAC-запрос и не ждать
	// боевые тайминги.
	startElevatedWorker = startElevated
	workerPollInterval  = 250 * time.Millisecond
	workerCancelWait    = 30 * time.Second
)

// workerStateReadRetries — сколько тиков подряд чтение state.json может
// падать, прежде чем это считается сбоем, а не подменой файла воркером.
const workerStateReadRetries = 8

type workerHandle interface {
	wait() (int, error)
	close()
	terminate() error
}

type elevatedResult struct {
	code int
	err  error
}

// Установщик сам по себе лаунчеру больше не принадлежит: вместо того чтобы
// поднимать его напрямую через ShellExecuteEx и держать в подвешенном
// состоянии до смерти процесса, лаунчер один раз поднимает собственный
// воркер (RunWorker в worker_run.go) с правами администратора и дальше
// общается с ним только через файлы spec/state/cancel. Воркер сам владеет
// установщиком, держит его в job-объекте и умеет прерывать разведку
// компонентов — то, что раньше было недостижимо для процесса, которым
// лаунчер не управляет.
func runElevated(ctx context.Context, spec runSpec) (int, error) {
	slog.Info("installer requires elevation", "path", spec.Path, "background", spec.Background)
	if spec.StatePath == "" {
		return 0, errWorkerStatePath
	}
	dir := filepath.Dir(spec.StatePath)
	specFile := workerSpecFilePath(dir, spec.ID)

	if err := clearWorkerCancel(spec.CancelPath); err != nil {
		return 0, fmt.Errorf("подготовка воркера установки: %w", err)
	}
	run := newID()
	ws := workerSpec{
		ID:            spec.ID,
		Run:           run,
		InstallerPath: spec.InstallerPath,
		Engine:        spec.Engine,
		Destination:   spec.Destination,
		WorkingDir:    spec.Dir,
		LogPath:       spec.LogPath,
		InfPath:       spec.InfPath,
		StatePath:     spec.StatePath,
		CancelPath:    spec.CancelPath,
		Options:       spec.Options,
		Background:    spec.Background,
		Hidden:        true,
	}
	exited, cleanup, terminate, err := handOffToWorker(spec, ws, specFile)
	if err != nil {
		return 0, err
	}
	defer cleanup()

	ticker := time.NewTicker(workerPollInterval)
	defer ticker.Stop()

	cancelRequested := false
	cancelSent := false
	var cancelDeadline <-chan time.Time
	stateReadFailures := 0
	for {
		select {
		case res := <-exited:
			if res.err != nil {
				return 0, fmt.Errorf("%w: %w", errInstallerNotConfirmedStopped, res.err)
			}
			return readFinalWorkerState(spec.StatePath, run)
		case <-ctx.Done():
			if !cancelRequested {
				cancelRequested = true
				cancelDeadline = time.After(workerCancelWait)
				if err := writeWorkerCancel(spec.CancelPath); err != nil {
					slog.Warn("request installer worker cancellation", "path", spec.Path, "error", err)
				} else {
					cancelSent = true
				}
			}
		case <-cancelDeadline:
			// Дедлайн истёк, и воркер не подтвердил остановку сам. Раньше
			// здесь просто возвращалась ошибка, а defer cleanup() закрывал
			// хэндл (CloseHandle на Windows), не трогая сам процесс —
			// воркер с правами администратора продолжал жить. terminate() —
			// тот же elevatedProc.terminate (TerminateProcess), которым
			// раньше пользовался только его собственный тест; теперь он
			// действительно убивает воркер, и это закрывает утечку процесса.
			//
			// Но класс ошибки НЕ меняется даже при успешном terminate():
			// убитый воркер не значит убитый установщик. Воркер держит
			// установщик живым через job-объект с
			// JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE (runner_windows.go,
			// limitJob), а SetInformationJobObject может отказать
			// (там же — «отказ воспроизведён на этой машине, похоже на
			// вмешательство защитного ПО») и limitJob в этом случае молча
			// откатывается на лимиты без этого флага. Значит подтверждённая
			// смерть воркера не доказывает смерть дерева процессов, которое
			// он запустил, и discardSilent (flow.go) обязан остаться в
			// консервативной ветке: RemoveAll по каталогу, в который ещё
			// может писать не убитый установщик, — гонка на единственной
			// копии данных (инвариант 9). Цена — каталог отменённой
			// установки остаётся на диске после принудительного убийства;
			// это осознанно и совпадает с поведением до этого фикса.
			if killErr := terminate(); killErr != nil {
				slog.Warn("kill installer worker after cancel timeout", "path", spec.Path, "error", killErr)
			}
			return 0, fmt.Errorf("%w: %w", errInstallerNotConfirmedStopped, ctx.Err())
		case <-ticker.C:
			if cancelRequested && !cancelSent {
				// Один транзиентный отказ записи (антивирус держит хэндл,
				// EACCES) не должен навсегда снять попытки: без ретрая
				// воркер никогда не узнаёт об отмене, и итоговая ошибка
				// неотличима от «воркер не успел ответить». Повтор идёт с
				// темпом опроса состояния — тем же, что и до этого момента, —
				// а не в цикле на каждый ctx.Done(), который остаётся
				// готовым (и потому выбираемым select) после первого срабатывания.
				if err := writeWorkerCancel(spec.CancelPath); err != nil {
					slog.Debug("retry installer worker cancellation", "path", spec.Path, "error", err)
				} else {
					cancelSent = true
				}
			}
			state, found, stateErr := readWorkerState(spec.StatePath)
			if stateErr != nil {
				// Воркер подменяет state.json переименованием, и на Windows
				// чтение ровно в этот момент получает ERROR_SHARING_VIOLATION.
				// Это «ещё не готово», а не сбой установки, поэтому одиночная
				// ошибка стоит следующего тика, а не отказа. Ошибка, которая
				// не проходит workerStateReadRetries тиков подряд, — уже не
				// подмена файла, и вот её мы возвращаем.
				stateReadFailures++
				if stateReadFailures >= workerStateReadRetries {
					return 0, fmt.Errorf("%w: состояние установки: %w", errInstallerNotConfirmedStopped, stateErr)
				}
				slog.Debug("read installer worker state", "path", spec.StatePath,
					"attempt", stateReadFailures, "error", stateErr)
				continue
			}
			stateReadFailures = 0
			if found && state.Done && state.Run == run {
				return finishElevatedState(state)
			}
		}
	}
}

// errBrokerTerminateUnsupported — брокер переживает одну установку: цепочка
// установщиков одной игры идёт через тот же процесс, и убить его здесь
// значило бы потребовать новый UAC на следующем установщике очереди, ровно
// тогда, когда пользователя перед экраном уже может не быть. Раз убить
// нечего, runElevated обязан остаться на консервативной ветке
// errInstallerNotConfirmedStopped — брокер не наш, чтобы решать за него.
var errBrokerTerminateUnsupported = errors.New("процесс, которым владеет брокер установки, нельзя прервать отсюда")

// handOffToWorker отдаёт задание либо уже поднятому брокеру, либо свежему
// воркеру. Канал в обоих случаях значит одно: процесс, которому отдали
// установку, больше не работает, и финальное состояние надо читать с диска.
// terminate — способ runElevated принудительно оборвать ожидание, если
// воркер не подтвердил остановку к дедлайну; для брокера его нет (см.
// errBrokerTerminateUnsupported).
func handOffToWorker(spec runSpec, ws workerSpec, specFile string) (<-chan elevatedResult, func(), func() error, error) {
	if spec.Broker != nil {
		if err := writeSignedBrokerSpec(spec.Broker.Dir, ws, spec.Broker.Key); err != nil {
			return nil, nil, nil, fmt.Errorf("передача задания брокеру установки: %w", err)
		}
		gone := spec.Broker.Gone
		exited := make(chan elevatedResult, 1)
		// stop нужен потому, что брокер переживает одну установку: цепочка
		// установщиков идёт через него же, и без выхода по cleanup эта
		// горутина оставалась бы висеть до его смерти (инвариант 19).
		stop := make(chan struct{})
		go func() {
			select {
			case <-gone:
				exited <- elevatedResult{}
			case <-stop:
			}
		}()
		terminate := func() error { return errBrokerTerminateUnsupported }
		return exited, func() { close(stop) }, terminate, nil
	}

	if err := writeWorkerSpec(specFile, ws); err != nil {
		return nil, nil, nil, fmt.Errorf("подготовка воркера установки: %w", err)
	}
	exe, err := os.Executable()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("путь к лаунчеру: %w", err)
	}
	proc, err := startElevatedWorker(runSpec{Path: exe, Args: []string{installWorkerFlag, specFile}, Hidden: true})
	if err != nil {
		return nil, nil, nil, workerStartError(spec.Path, err)
	}
	exited := make(chan elevatedResult, 1)
	go func() {
		// Хэндл закрывает только тот, кто его ждёт: CloseHandle под висящим
		// WaitForSingleObject из другой горутины — это ожидание на
		// переиспользованном значении (та же дисциплина, что в
		// broker_host.go, см. комментарий у tendBroker). runElevated может
		// вернуться раньше — по cancelDeadline или по ошибке воркера, —
		// поэтому close() не может быть частью cleanup(), вызываемого из её
		// собственной горутины; он ждёт здесь же, сколько бы это ни заняло.
		defer proc.close()
		code, waitErr := proc.wait()
		exited <- elevatedResult{code: code, err: waitErr}
	}()
	return exited, func() {}, proc.terminate, nil
}

func readFinalWorkerState(statePath, run string) (int, error) {
	state, found, err := readWorkerState(statePath)
	if err != nil {
		return 0, fmt.Errorf("%w: состояние установки: %w", errInstallerNotConfirmedStopped, err)
	}
	if !found || !state.Done || state.Run != run {
		return 0, fmt.Errorf("%w: %w", errInstallerNotConfirmedStopped, errWorkerNotFinished)
	}
	return finishElevatedState(state)
}

// finishElevatedState оборачивает отмену через %w вокруг context.Canceled:
// Service.fail (service.go) решает, что делать с ошибкой, через
// errors.Is(cause, context.Canceled), а errors.New(state.Error) создаёт
// значение, не сравнимое ни с чем (инвариант 24 — отмена и провал
// установщика не одна и та же причина, и это должно быть видно вызывающему,
// а не только человеку, читающему текст).
func finishElevatedState(state workerState) (int, error) {
	switch {
	case state.Cancelled:
		return state.Code, fmt.Errorf("установка отменена: %w", context.Canceled)
	case state.Error != "":
		return state.Code, errors.New(state.Error)
	default:
		return state.Code, nil
	}
}

package install

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"

	"typhon/internal/storage"
)

type workerPhase string

const (
	workerPhaseDiscovering workerPhase = "discovering"
	workerPhaseInstalling  workerPhase = "installing"
)

var errWorkerStatePathUnavailable = errors.New("путь состояния воркера не задан")

// workerSpec — всё, что нужно повышенному воркеру, чтобы самостоятельно
// провести (при необходимости) разведку компонентов Inno и запустить основной
// тихий прогон установщика: воркер получает только эти данные, остальное
// состояние сервиса ему недоступно и не нужно.
type workerSpec struct {
	BrokerSignature string `json:"brokerSignature,omitempty"`
	InstallerSHA256 string `json:"installerSHA256,omitempty"`
	ID              string `json:"id"`
	// Run отличает прогоны одного установочного задания. Файл состояния один
	// на всю цепочку установщиков, и без него оставшийся от предыдущего
	// Done: true был бы прочитан как результат следующего прогона: воркер
	// перезаписывает файл не мгновенно, а лаунчер начинает опрос сразу.
	Run           string         `json:"run,omitempty"`
	InstallerPath string         `json:"installerPath"`
	Engine        Engine         `json:"engine"`
	Destination   string         `json:"destination"`
	WorkingDir    string         `json:"workingDir"`
	LogPath       string         `json:"logPath"`
	InfPath       string         `json:"infPath"`
	StatePath     string         `json:"statePath"`
	CancelPath    string         `json:"cancelPath"`
	Options       installOptions `json:"options"`
	Background    bool           `json:"background"`
	Hidden        bool           `json:"hidden"`
	// Interactive просит воркер запустить сам установщик без ключей тишины,
	// с видимым окном и без разведки компонентов: пользователь проходит мастер
	// сам, воркер нужен только ради прав администратора.
	Interactive bool      `json:"interactive"`
	Shell       *shellJob `json:"shell,omitempty"`
}

// discoverySpec — минимальный набор полей, нужных именно для разведки
// компонентов Inno. workerSpec (воркер, уже с правами администратора) и
// runSpec (обычный неэлевированный путь запуска, runner_windows.go) сводятся
// к нему через discovery(), чтобы attemptDiscovery оставался одной функцией
// на оба пути (инвариант 28), а не была продублирована под каждый спек.
type discoverySpec struct {
	Engine        Engine
	InstallerPath string
	Destination   string
	WorkingDir    string
	InfPath       string
	Options       installOptions
	// Interactive отключает разведку: она существует только ради тихого
	// прогона, а мастер установщика пользователь проходит сам.
	Interactive bool
}

func (s workerSpec) discovery() discoverySpec {
	return discoverySpec{
		Engine: s.Engine, InstallerPath: s.InstallerPath, Destination: s.Destination,
		WorkingDir: s.WorkingDir, InfPath: s.InfPath, Options: s.Options, Interactive: s.Interactive,
	}
}

// workerState — единственный канал, которым воркер сообщает о себе лаунчеру:
// оба процесса общаются только через файл, поэтому Done обязан выставляться
// строго в последнюю запись, когда Code и Error уже финальны. Cancelled
// хранится отдельным флагом, а не только текстом Error: runner_windows.go
// должен уметь вернуть ошибку, для которой errors.Is(err, context.Canceled)
// истинно, а сравнение по errors.New(text) с этим не справится (инвариант 24 —
// отмена и провал не одна и та же причина).
type workerState struct {
	PID              int      `json:"pid"`
	Run              string   `json:"run,omitempty"`
	Phase            string   `json:"phase"`
	Code             int      `json:"code"`
	Done             bool     `json:"done"`
	Error            string   `json:"error"`
	Cancelled        bool     `json:"cancelled,omitempty"`
	Components       []string `json:"components,omitempty"`
	DiscoveryFailure string   `json:"discoveryFailure,omitempty"`
	// Shell заполнен, когда уборку ярлыков просили и установщик отработал; nil
	// значит, что не просили или запуск установщика вернул ошибку. Установщик,
	// завершившийся неуспехом, даёт Skipped.
	Shell *shellReport `json:"shell,omitempty"`
}

func workerStatePath(dir, id string) string {
	return filepath.Join(dir, "worker-"+id+"-state.json")
}

func workerSpecFilePath(dir, id string) string {
	return filepath.Join(dir, "worker-"+id+"-spec.json")
}

func workerInfPath(dir, id string) string {
	return filepath.Join(dir, "worker-"+id+"-discover.ini")
}

func workerCancelPath(dir, id string) string {
	return filepath.Join(dir, "worker-"+id+"-cancel")
}

func readWorkerSpec(path string) (workerSpec, error) {
	data, err := readWorkerSpecBytes(path)
	if err != nil {
		return workerSpec{}, err
	}
	var spec workerSpec
	if err := json.Unmarshal(data, &spec); err != nil {
		return spec, fmt.Errorf("parse worker spec %s: %w", path, err)
	}
	return spec, nil
}

func writeWorkerSpec(path string, spec workerSpec) error {
	_, err := writeWorkerSpecDigest(path, spec)
	return err
}

// writeWorkerSpecDigest возвращает SHA-256 тех самых байт, что ушли в файл, а
// не файла, перечитанного с диска: между записью и чтением его уже мог
// подменить другой процесс.
func writeWorkerSpecDigest(path string, spec workerSpec) (string, error) {
	if path == "" {
		return "", errors.New("worker spec path unavailable")
	}
	data, err := json.MarshalIndent(spec, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal worker spec: %w", err)
	}
	if err := writeWorkerFile(path, data); err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

// readWorkerState различает только отсутствие файла: воркер ещё не успел его
// создать. Любая другая ошибка чтения — реальный сбой, который не превращается
// в "воркер не отвечал".
func readWorkerState(path string) (workerState, bool, error) {
	if path == "" {
		return workerState{}, false, errWorkerStatePathUnavailable
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return workerState{}, false, nil
	}
	if err != nil {
		return workerState{}, false, fmt.Errorf("read worker state %s: %w", path, err)
	}
	var state workerState
	if err := json.Unmarshal(data, &state); err != nil {
		return workerState{}, false, fmt.Errorf("parse worker state %s: %w", path, err)
	}
	return state, true, nil
}

func writeWorkerState(path string, state workerState) error {
	if path == "" {
		return errWorkerStatePathUnavailable
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal worker state: %w", err)
	}
	return writeWorkerFile(path, data)
}

func writeWorkerFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("prepare worker file dir %s: %w", filepath.Dir(path), err)
	}
	return storage.WriteAtomic(path, data)
}

func writeWorkerCancel(path string) error {
	if path == "" {
		return errors.New("worker cancel path unavailable")
	}
	return writeWorkerFile(path, []byte{})
}

// removeWorkerFiles убирает файлы прогона воркера. Отсутствие файла не ошибка,
// любая другая причина возвращается: оставшееся состояние прошлого прогона
// иначе принималось бы за итог следующего.
// A parent that is not a directory answers ENOTDIR on POSIX and "path not
// found" on Windows; either way the file cannot be there.
func alreadyGone(err error) bool {
	return errors.Is(err, fs.ErrNotExist) || errors.Is(err, syscall.ENOTDIR)
}

func removeWorkerFiles(paths ...string) error {
	var errs []error
	for _, path := range paths {
		if path == "" {
			continue
		}
		if err := os.Remove(path); err != nil && !alreadyGone(err) {
			errs = append(errs, fmt.Errorf("remove worker file %s: %w", path, err))
		}
	}
	return errors.Join(errs...)
}

func clearWorkerCancel(path string) error {
	if path == "" {
		return nil
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("remove worker cancel marker %s: %w", path, err)
	}
	return nil
}

func workerCancelRequested(path string) bool {
	if path == "" {
		return false
	}
	_, err := os.Stat(path)
	return err == nil
}

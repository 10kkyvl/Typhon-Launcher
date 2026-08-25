package install

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

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
	ID            string         `json:"id"`
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
}

func (s workerSpec) discovery() discoverySpec {
	return discoverySpec{
		Engine: s.Engine, InstallerPath: s.InstallerPath, Destination: s.Destination,
		WorkingDir: s.WorkingDir, InfPath: s.InfPath, Options: s.Options,
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
	Phase            string   `json:"phase"`
	Code             int      `json:"code"`
	Done             bool     `json:"done"`
	Error            string   `json:"error"`
	Cancelled        bool     `json:"cancelled,omitempty"`
	Components       []string `json:"components,omitempty"`
	DiscoveryFailure string   `json:"discoveryFailure,omitempty"`
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
	data, err := os.ReadFile(path)
	if err != nil {
		return workerSpec{}, fmt.Errorf("read worker spec %s: %w", path, err)
	}
	var spec workerSpec
	if err := json.Unmarshal(data, &spec); err != nil {
		return spec, fmt.Errorf("parse worker spec %s: %w", path, err)
	}
	return spec, nil
}

func writeWorkerSpec(path string, spec workerSpec) error {
	if path == "" {
		return errors.New("worker spec path unavailable")
	}
	data, err := json.MarshalIndent(spec, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal worker spec: %w", err)
	}
	return writeWorkerFile(path, data)
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

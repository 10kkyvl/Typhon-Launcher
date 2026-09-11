package install

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"typhon/internal/uierr"
)

const installBrokerFlag = "--install-broker"

// Outcome of a broker process. Only Installed did any work; the rest are the
// designed ways for an idle elevated process to go away, and none of them is
// a failure of the install.
type BrokerOutcome string

const (
	BrokerInstalled BrokerOutcome = "installed"
	BrokerAborted   BrokerOutcome = "aborted"
	BrokerAbandoned BrokerOutcome = "abandoned"
	BrokerExpired   BrokerOutcome = "expired"
)

var (
	errBrokerNoPin      = errors.New("каталог брокера установки без файла границ")
	errBrokerOutsidePin = uierr.New("install.broker_outside_pin", "задание установки вышло за границы, согласованные при запросе прав")
	errBrokerNoDir      = errors.New("--install-broker требует путь к каталогу")

	// Подменяются в тестах, чтобы не ждать боевые тайминги.
	brokerPollInterval    = 500 * time.Millisecond
	brokerHeartbeatPeriod = 30 * time.Second
	brokerHeartbeatGrace  = 2 * time.Minute
	brokerMaxLifetime     = 12 * time.Hour
)

// brokerPin restricts paths; signed jobs authenticate the parent separately.
type brokerPin struct {
	DownloadID  string `json:"downloadId"`
	ContentRoot string `json:"contentRoot"`
	LibraryRoot string `json:"libraryRoot"`
	StateDir    string `json:"stateDir"`
}

func brokerPinPath(dir string) string   { return filepath.Join(dir, "pin.json") }
func brokerSpecPath(dir string) string  { return filepath.Join(dir, "spec.json") }
func brokerAlivePath(dir string) string { return filepath.Join(dir, "alive") }
func brokerAbortPath(dir string) string { return filepath.Join(dir, "abort") }

// brokerStateAnchor существует только чтобы devmock-запуск знал, куда класть
// лог брокера: startElevated берёт каталог из StatePath.
func brokerStateAnchor(dir string) string { return filepath.Join(dir, "broker-state.json") }

func writeBrokerPin(path string, pin brokerPin) error {
	data, err := json.MarshalIndent(pin, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal broker pin: %w", err)
	}
	return writeWorkerFile(path, data)
}

func readBrokerPin(path string) (brokerPin, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return brokerPin{}, fmt.Errorf("%w: %s", errBrokerNoPin, path)
	}
	if err != nil {
		return brokerPin{}, fmt.Errorf("read broker pin %s: %w", path, err)
	}
	var pin brokerPin
	if err := json.Unmarshal(data, &pin); err != nil {
		return brokerPin{}, fmt.Errorf("parse broker pin %s: %w", path, err)
	}
	if pin.ContentRoot == "" || pin.LibraryRoot == "" || pin.StateDir == "" {
		return brokerPin{}, fmt.Errorf("%w: %s", errBrokerNoPin, path)
	}
	return pin, nil
}

func touchBrokerAlive(path string) error {
	now := time.Now()
	err := os.Chtimes(path, now, now)
	if errors.Is(err, fs.ErrNotExist) {
		return writeWorkerFile(path, []byte{})
	}
	if err != nil {
		return fmt.Errorf("touch broker heartbeat %s: %w", path, err)
	}
	return nil
}

// brokerAbandoned отвечает «да» только на протухший хартбит. Ошибка чтения —
// не признак смерти лаунчера, и превращать её в выход означало бы отпустить
// права ровно тогда, когда установка ещё может их попросить.
func brokerAbandoned(path string, grace time.Duration) (bool, error) {
	info, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return true, nil
	}
	if err != nil {
		return false, fmt.Errorf("stat broker heartbeat %s: %w", path, err)
	}
	return time.Since(info.ModTime()) > grace, nil
}

func brokerAbortRequested(path string) (bool, error) {
	_, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("stat broker abort marker %s: %w", path, err)
	}
	return true, nil
}

// validateBrokerSpec — единственное, что стоит между правами администратора и
// произвольным исполняемым файлом: спеку пишет непривилегированный процесс,
// поэтому каждый путь из неё проверяется против границ, согласованных до
// повышения.
func validateBrokerSpec(pin brokerPin, spec workerSpec) error {
	within := []struct {
		root string
		path string
	}{
		{pin.ContentRoot, spec.InstallerPath},
		{pin.ContentRoot, spec.WorkingDir},
		{pin.LibraryRoot, spec.Destination},
		{pin.StateDir, spec.StatePath},
		{pin.StateDir, spec.CancelPath},
		{pin.StateDir, spec.LogPath},
		{pin.StateDir, spec.InfPath},
	}
	for _, pair := range within {
		if pair.path == "" {
			continue
		}
		if !inside(pair.root, pair.path) && !samePath(pair.root, pair.path) {
			return fmt.Errorf("%w: %s вне %s", errBrokerOutsidePin, pair.path, pair.root)
		}
	}
	if spec.InstallerPath == "" {
		return fmt.Errorf("%w: задание без пути установщика", errBrokerOutsidePin)
	}
	return nil
}

func readBrokerSpec(dir string, pin brokerPin, key ...string) (workerSpec, bool, error) {
	path := brokerSpecPath(dir)
	if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
		return workerSpec{}, false, nil
	} else if err != nil {
		return workerSpec{}, false, fmt.Errorf("stat broker spec %s: %w", path, err)
	}
	spec, err := readWorkerSpec(path)
	if err != nil {
		return workerSpec{}, false, err
	}
	if err := validateBrokerSpec(pin, spec); err != nil {
		return workerSpec{}, false, err
	}
	if err := verifyBrokerSignature(spec, key); err != nil {
		return workerSpec{}, false, err
	}
	return spec, true, nil
}

// RunBroker — точка входа процесса, который лаунчер поднял с правами
// администратора заранее, пока пользователь ещё был у экрана. Он ничего не
// делает, пока не появится задание, и умирает сам, если лаунчер о нём забыл:
// висящий процесс с высоким уровнем целостности и есть цена этой схемы, и
// ограничивать её время жизни больше некому.
//
//nolint:forbidigo // RunBroker — точка входа отдельного процесса, эквивалент main для брокера: вызывающего ctx нет (инвариант 20 разрешает Background только в main)
func RunBroker(dir string, key ...string) (BrokerOutcome, error) {
	if dir == "" {
		return BrokerAborted, errBrokerNoDir
	}
	pin, err := readBrokerPin(brokerPinPath(dir))
	if err != nil {
		return BrokerAborted, err
	}

	deadline := time.Now().Add(brokerMaxLifetime)
	ticker := time.NewTicker(brokerPollInterval)
	defer ticker.Stop()

	// Один установщик — одно задание, но цепочка установщиков одной игры
	// проходит через того же брокера: выйти после первого значило бы
	// потребовать UAC на втором, ровно тогда, когда пользователя уже нет.
	served := 0
	seen := map[string]bool{}
	for {
		aborted, err := brokerAbortRequested(brokerAbortPath(dir))
		if err != nil {
			return brokerIdleOutcome(served, BrokerAborted), err
		}
		if aborted {
			return brokerIdleOutcome(served, BrokerAborted), nil
		}

		spec, found, err := readBrokerSpec(dir, pin, key...)
		if err != nil {
			return brokerIdleOutcome(served, BrokerAborted), err
		}
		if found {
			if spec.Run == "" || seen[spec.Run] {
				return BrokerAborted, errBrokerOutsidePin
			}
			seen[spec.Run] = true
			// Задание снимается до запуска: иначе смерть брокера посреди
			// установки оставила бы спеку, которую следующий прогон принял
			// бы за новую и поставил игру второй раз.
			if err := os.Remove(brokerSpecPath(dir)); err != nil {
				return brokerIdleOutcome(served, BrokerAborted), fmt.Errorf("consume broker spec: %w", err)
			}
			slog.Info("broker takes the install", "download", pin.DownloadID, "installer", spec.InstallerPath)
			if err := runSignedBrokerSpec(spec); err != nil {
				return BrokerInstalled, err
			}
			served++
			continue
		}

		abandoned, err := brokerAbandoned(brokerAlivePath(dir), brokerHeartbeatGrace)
		if err != nil {
			return brokerIdleOutcome(served, BrokerAborted), err
		}
		if abandoned {
			return brokerIdleOutcome(served, BrokerAbandoned), nil
		}
		if !time.Now().Before(deadline) {
			return brokerIdleOutcome(served, BrokerExpired), nil
		}
		<-ticker.C
	}
}

func brokerIdleOutcome(served int, idle BrokerOutcome) BrokerOutcome {
	if served > 0 {
		return BrokerInstalled
	}
	return idle
}

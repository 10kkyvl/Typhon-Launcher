package install

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"time"
)

var (
	errShellNoRoots      = errors.New("общие каталоги ярлыков не определены")
	errShellOutsideRoots = errors.New("запись снимка ярлыков лежит вне общих каталогов")
	errShellBadTarget    = errors.New("каталог игры для уборки ярлыков задан не абсолютным путём")
	errShellNoTarget     = errors.New("каталог игры для уборки ярлыков не задан")
	errShellBroadTarget  = errors.New("каталог игры для уборки ярлыков охватывает чужие файлы")

	// Подменяются в тестах: настоящие известные папки Windows трогать нельзя.
	shortcutRootsFn = shortcutRoots
	sharedRootsFn   = sharedShortcutRoots
	systemFoldersFn = systemFolders
)

// Время хранится секундами и наносекундами, а не одним UnixNano: тот
// переполняется вне 1678–2262 годов, а файлы из архивов бывают и 1601 года.
type shellEntryWire struct {
	Dir     bool  `json:"dir,omitempty"`
	Size    int64 `json:"size"`
	ModSec  int64 `json:"modSec"`
	ModNsec int64 `json:"modNsec,omitempty"`
}

func wireShellEntry(entry shellEntry) shellEntryWire {
	return shellEntryWire{Dir: entry.dir, Size: entry.size, ModSec: entry.mod.Unix(), ModNsec: int64(entry.mod.Nanosecond())}
}

// shellJob — всё, что лаунчер передаёт повышенному воркеру для уборки общих
// каталогов: снимок «до» и флаг game. Сами каталоги воркер определяет сам, а
// цель игры берёт из Destination спеки, которую брокер уже проверил.
type shellJob struct {
	Before map[string]shellEntryWire `json:"before"`
	Game   bool                      `json:"game"`
}

// shellReport — итог уборки, который воркер кладёт в файл состояния рядом с
// Done: лаунчер мог перезапуститься, пока воркер работал, и читать итог
// приходится из файла. Skipped — установщик завершился, но не успешно, и
// уборку воркер не начинал.
type shellReport struct {
	Removed []string `json:"removed,omitempty"`
	Skipped bool     `json:"skipped,omitempty"`
	Error   string   `json:"error,omitempty"`
}

// shellHandoff — у каждого установщика цепочки свой: Job уходит воркеру, а
// Report заполняет runElevated, когда воркер дописал состояние. Report == nil
// значит, что этот установщик шёл без воркера и общие каталоги после него
// остаются за лаунчером. shared — каталоги, под которые собран Job.
type shellHandoff struct {
	Job    shellJob
	Report *shellReport
	shared []string
}

func newShellHandoff(before shellSnapshot, game bool) (*shellHandoff, error) {
	if !before.taken {
		return nil, nil
	}
	shared, err := sharedRootsFn()
	if err != nil {
		return nil, err
	}
	if len(shared) == 0 {
		return nil, nil
	}
	job := shellJob{Game: game, Before: make(map[string]shellEntryWire)}
	for key, entry := range before.entries {
		if !insideAny(shared, entry.path) {
			continue
		}
		job.Before[key] = wireShellEntry(entry)
	}
	return &shellHandoff{Job: job, shared: shared}, nil
}

func (h *shellHandoff) forInstaller() *shellHandoff {
	if h == nil {
		return nil
	}
	return &shellHandoff{Job: h.Job, shared: h.shared}
}

func (h *shellHandoff) record(state workerState) {
	if h == nil || state.Shell == nil {
		return
	}
	report := *state.Shell
	h.Report = &report
}

// delegatedRoots отдаёт общие каталоги воркеру, только если после каждого
// установщика цепочки их убрал воркер: неэлевированный установщик той же
// цепочки мог положить ярлык в общий рабочий стол, а его воркер не видел.
func delegatedRoots(workers []*shellHandoff) []string {
	if len(workers) == 0 {
		return nil
	}
	for _, worker := range workers {
		if worker == nil || worker.Report == nil || worker.Report.Skipped {
			return nil
		}
	}
	return workers[0].shared
}

func workerReports(workers []*shellHandoff) []shellReport {
	reports := make([]shellReport, 0, len(workers))
	for _, worker := range workers {
		if worker != nil && worker.Report != nil {
			reports = append(reports, *worker.Report)
		}
	}
	return reports
}

func insideAny(roots []string, path string) bool {
	for _, root := range roots {
		if root != "" && inside(root, path) {
			return true
		}
	}
	return false
}

// without оставляет в снимке только то, что лаунчер может убрать сам: общие
// каталоги вычёркиваются вместе с записями в них.
func (snap shellSnapshot) without(shared []string) shellSnapshot {
	out := shellSnapshot{entries: make(map[string]shellEntry, len(snap.entries)), taken: snap.taken}
	for _, root := range snap.roots {
		if root == "" {
			continue
		}
		skip := false
		for _, other := range shared {
			if samePath(root, other) {
				skip = true
				break
			}
		}
		if !skip {
			out.roots = append(out.roots, root)
		}
	}
	for key, entry := range snap.entries {
		if !insideAny(shared, entry.path) {
			out.entries[key] = entry
		}
	}
	return out
}

// shellKeyInside сверяет ключ снимка с корнями по тексту: ключи пишутся в
// нижнем регистре, а spec приходит из файла, поэтому ".." и относительные пути
// отсекаются до всякого обращения к диску.
func shellKeyInside(roots []string, key string) bool {
	if !filepath.IsAbs(key) || filepath.Clean(key) != key {
		return false
	}
	key = strings.ToLower(key)
	for _, root := range roots {
		if root == "" {
			continue
		}
		prefix := strings.TrimSuffix(strings.ToLower(filepath.Clean(root)), string(filepath.Separator)) + string(filepath.Separator)
		if strings.HasPrefix(key, prefix) {
			return true
		}
	}
	return false
}

func (j shellJob) snapshot(roots []string) (shellSnapshot, error) {
	if len(j.Before) > shortcutMaxEntries {
		return shellSnapshot{}, errShellTooLarge
	}
	snap := shellSnapshot{roots: roots, entries: make(map[string]shellEntry, len(j.Before)), taken: true}
	for key, wire := range j.Before {
		if !shellKeyInside(roots, key) {
			return shellSnapshot{}, fmt.Errorf("%w: %s", errShellOutsideRoots, key)
		}
		snap.entries[key] = shellEntry{dir: wire.Dir, size: wire.Size, mod: time.Unix(wire.ModSec, wire.ModNsec)}
	}
	return snap, nil
}

// cleanSharedShortcuts выполняется в процессе с правами администратора и
// удаляет только внутри общих каталогов, которые определяет сам: из spec
// берутся снимок «до» (он проверяется против этих каталогов), флаг game и цель.
// Цель, под которую попали бы чужие ярлыки, выключает правило игры, но не
// уборку ярлыка сайта, и попадает в итог ошибкой.
func cleanSharedShortcuts(ctx context.Context, job shellJob, target string) shellReport {
	if target != "" && !filepath.IsAbs(target) {
		return shellReport{Error: fmt.Errorf("%w: %s", errShellBadTarget, target).Error()}
	}
	roots, err := sharedRootsFn()
	if err != nil {
		return shellReport{Error: err.Error()}
	}
	if len(roots) == 0 {
		return shellReport{Error: errShellNoRoots.Error()}
	}
	before, err := job.snapshot(roots)
	if err != nil {
		return shellReport{Error: err.Error()}
	}
	game := job.Game
	var targetErr error
	if game {
		if targetErr = checkGameTarget(target, roots); targetErr != nil {
			game = false
		}
	}
	removed, err := cleanShellShortcuts(ctx, before, target, game)
	report := shellReport{Removed: removed}
	if err := errors.Join(targetErr, err); err != nil {
		report.Error = err.Error()
	}
	return report
}

// checkGameTarget не пускает в правило игры каталог, путь которого встречается
// в чужих ярлыках: корень тома, каталог первого уровня, системный каталог, сам
// Program Files и общие каталоги ярлыков. Spec без брокера не подписан, и
// такой Destination в нём — удаление чужих ярлыков с правами администратора.
func checkGameTarget(target string, shared []string) error {
	switch {
	case target == "":
		return errShellNoTarget
	case !filepath.IsAbs(target) || filepath.Clean(target) != target:
		return fmt.Errorf("%w: %s", errShellBadTarget, target)
	case pathDepth(target) < 2:
		return fmt.Errorf("%w: %s", errShellBroadTarget, target)
	}
	windir, programFiles, err := systemFoldersFn()
	if err != nil {
		return fmt.Errorf("%w: системные каталоги не определены: %w", errShellBroadTarget, err)
	}
	if windir != "" && inside(windir, target) {
		return fmt.Errorf("%w: %s внутри %s", errShellBroadTarget, target, windir)
	}
	for _, dir := range programFiles {
		if dir != "" && samePath(dir, target) {
			return fmt.Errorf("%w: %s", errShellBroadTarget, target)
		}
	}
	for _, root := range shared {
		if root != "" && (inside(root, target) || inside(target, root)) {
			return fmt.Errorf("%w: %s пересекается с %s", errShellBroadTarget, target, root)
		}
	}
	return nil
}

func pathDepth(path string) int {
	rest := path[len(filepath.VolumeName(path)):]
	return len(strings.FieldsFunc(rest, func(r rune) bool { return r == '\\' || r == '/' }))
}

func logShellReports(id string, reports []shellReport) {
	for _, report := range reports {
		if report.Skipped {
			slog.Info("elevated worker left installer shortcuts alone, installer did not succeed", "id", id)
		}
		if len(report.Removed) > 0 {
			slog.Info("installer shortcuts removed by elevated worker", "id", id, "count", len(report.Removed), "paths", report.Removed)
		}
		if report.Error != "" {
			slog.Warn("elevated worker could not remove installer shortcuts", "id", id, "removed", len(report.Removed), "error", report.Error)
		}
	}
}

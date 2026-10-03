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

	// Подменяются в тестах: настоящие известные папки Windows трогать нельзя.
	shortcutRootsFn = shortcutRoots
	sharedRootsFn   = sharedShortcutRoots
)

type shellEntryWire struct {
	Dir  bool  `json:"dir,omitempty"`
	Size int64 `json:"size"`
	Mod  int64 `json:"mod"`
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
// приходится из файла.
type shellReport struct {
	Removed []string `json:"removed,omitempty"`
	Error   string   `json:"error,omitempty"`
}

// shellHandoff живёт в runSpec: Job уходит воркеру, а Reports заполняет
// runElevated, когда воркер дописал состояние. Пустые Reports значат, что
// установщик шёл без воркера и общие каталоги остаются за лаунчером.
type shellHandoff struct {
	Job     shellJob
	Reports []shellReport
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
		job.Before[key] = shellEntryWire{Dir: entry.dir, Size: entry.size, Mod: entry.mod.UnixNano()}
	}
	return &shellHandoff{Job: job}, nil
}

func (h *shellHandoff) record(state workerState) {
	if h == nil || state.Shell == nil {
		return
	}
	h.Reports = append(h.Reports, *state.Shell)
}

func (h *shellHandoff) reports() []shellReport {
	if h == nil {
		return nil
	}
	return h.Reports
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
		snap.entries[key] = shellEntry{dir: wire.Dir, size: wire.Size, mod: time.Unix(0, wire.Mod)}
	}
	return snap, nil
}

// cleanSharedShortcuts выполняется в процессе с правами администратора и
// удаляет только внутри общих каталогов, которые определяет сам: из spec
// берутся снимок «до» (он проверяется против этих каталогов), флаг game и цель.
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
	removed, err := cleanShellShortcuts(ctx, before, target, job.Game)
	report := shellReport{Removed: removed}
	if err != nil {
		report.Error = err.Error()
	}
	return report
}

func logShellReports(id string, reports []shellReport) {
	for _, report := range reports {
		if len(report.Removed) > 0 {
			slog.Info("installer shortcuts removed by elevated worker", "id", id, "count", len(report.Removed), "paths", report.Removed)
		}
		if report.Error != "" {
			slog.Warn("elevated worker could not remove installer shortcuts", "id", id, "removed", len(report.Removed), "error", report.Error)
		}
	}
}

package install

import (
	"context"
	"encoding/hex"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"testing"
	"time"

	"typhon/internal/settings"

	"github.com/wailsapp/wails/v3/pkg/application"
)

var uiCodePattern = regexp.MustCompile(`typhon:([a-z0-9_]+(?:\.[a-z0-9_]+)*)`)

// uiCode повторяет разбор кода на стороне интерфейса (frontend/src/lib/i18n/errors.ts):
// запись установки хранит ошибку текстом, и код живёт внутри него.
func uiCode(text string) string {
	match := uiCodePattern.FindStringSubmatch(text)
	if match == nil {
		return ""
	}
	return match[1]
}

func errCode(err error) string {
	if err == nil {
		return ""
	}
	return uiCode(err.Error())
}

// rig — сервис установки с каталогом состояния, который переживает перезапуск:
// restart поднимает новый Service над теми же файлами, как это делает лаунчер.
// Всё, что трогает Windows (ярлыки), направлено во временные каталоги.
type rig struct {
	t         *testing.T
	dir       string
	cfg       *settings.Service
	downloads *fakeDownloads
	reg       *fakeRegistrar
	lib       registrar
	games     string
	runner    runner
	s         *Service
	down      bool
}

func newRig(t *testing.T) *rig {
	t.Helper()
	r := &rig{
		t:         t,
		dir:       t.TempDir(),
		cfg:       newTestSettings(t),
		downloads: newFakeDownloads(),
		reg:       &fakeRegistrar{},
		games:     t.TempDir(),
	}
	cfg := r.cfg.GetSettings()
	cfg.InstallCleanupPolicy = settings.CleanupKeep
	if err := r.cfg.SaveSettings(cfg); err != nil {
		t.Fatalf("save settings: %v", err)
	}
	user, shared := t.TempDir(), t.TempDir()
	useSharedRoots(t, shared)
	previous := shortcutRootsFn
	shortcutRootsFn = func() ([]string, error) { return []string{user, shared}, nil }
	t.Cleanup(func() { shortcutRootsFn = previous })
	r.boot()
	t.Cleanup(r.shutdown)
	return r
}

func (r *rig) boot() {
	r.t.Helper()
	s := mustServiceAt(r.t, r.dir)
	s.settings = r.cfg
	s.downloads = r.downloads
	s.library = r.reg
	if r.lib != nil {
		s.library = r.lib
	}
	s.roots = []string{r.games}
	if r.runner != nil {
		s.runner = r.runner
	}
	if err := s.ServiceStartup(context.Background(), application.ServiceOptions{}); err != nil {
		r.t.Fatalf("startup: %v", err)
	}
	r.s = s
	r.down = false
}

func (r *rig) shutdown() {
	if r.s == nil || r.down {
		return
	}
	r.down = true
	if err := r.s.ServiceShutdown(); err != nil {
		r.t.Errorf("shutdown: %v", err)
	}
}

func (r *rig) restart() {
	r.t.Helper()
	r.shutdown()
	r.boot()
}

func (r *rig) setRunner(run runner) {
	r.runner = run
	r.s.runner = run
}

func (r *rig) setCleanup(policy string) {
	r.t.Helper()
	cfg := r.cfg.GetSettings()
	cfg.InstallCleanupPolicy = policy
	if err := r.cfg.SaveSettings(cfg); err != nil {
		r.t.Fatalf("save settings: %v", err)
	}
}

func isSettled(status Status) bool {
	switch status {
	case StatusCompleted, StatusFailed, StatusCancelled, StatusInterrupted, StatusWaitingForUser:
		return true
	}
	return false
}

// settle ждёт, пока установка придёт в состояние, из которого сама не выйдет, и
// пока её горутина отдаст запись о задании: после этого память и диск обязаны совпадать.
func (r *rig) settle(id string) Installation {
	r.t.Helper()
	waitFor(r.t, "install to settle", func() bool {
		item, ok := r.s.snapshot(id)
		return ok && isSettled(item.Status)
	})
	r.s.waitJobDone(r.t, id)
	return r.get(id)
}

func (r *rig) get(id string) Installation {
	r.t.Helper()
	item, ok := r.s.snapshot(id)
	if !ok {
		r.t.Fatalf("installation %s is gone", id)
	}
	return item
}

// disk читает installations.json напрямую, мимо сервиса.
func (r *rig) disk() []Installation {
	r.t.Helper()
	items, err := newStore(r.dir).load()
	if err != nil {
		r.t.Fatalf("read installations.json: %v", err)
	}
	return items
}

func (r *rig) diskItem(id string) Installation {
	r.t.Helper()
	for _, item := range r.disk() {
		if item.ID == id {
			return item
		}
	}
	r.t.Fatalf("installation %s is not in installations.json", id)
	return Installation{}
}

// stateFiles — всё, что лежит в каталоге состояния, кроме двух постоянных списков.
func (r *rig) stateFiles() []string {
	r.t.Helper()
	entries, err := os.ReadDir(r.dir)
	if err != nil {
		r.t.Fatalf("read state dir: %v", err)
	}
	var out []string
	for _, e := range entries {
		switch e.Name() {
		case "installations.json", "removals.json":
			continue
		}
		out = append(out, e.Name())
	}
	sort.Strings(out)
	return out
}

func (r *rig) download(id, name, root string) {
	r.downloads.add(id, name, root)
}

// view — то, что интерфейс получает о записи и что обязано пережить перезапуск.
type view struct {
	Type        Type
	Status      Status
	Mode        string
	Destination string
	Executable  string
	Installer   string
	Extras      int
	Engine      Engine
	Silent      bool
	Interactive bool
	ChainStep   int
	Owned       bool
	HasGame     bool
	Completed   bool
	Error       string
}

func viewOf(i Installation) view {
	return view{
		Type: i.Type, Status: i.Status, Mode: i.Mode, Destination: i.Destination, Executable: i.Executable,
		Installer: i.InstallerPath, Extras: len(i.ExtraInstallers), Engine: i.Engine, Silent: i.Silent,
		Interactive: i.Interactive, ChainStep: i.ChainStep, Owned: i.Owned, HasGame: i.GameID != "",
		Completed: i.CompletedAt != nil, Error: i.Error,
	}
}

// 7z, собранный 7-Zip с -m0=copy: Game/Game.exe и Game/data/content.pak.
// Настоящий архив нужен, потому что распаковщик 7z в пакете — внешняя
// библиотека, а не код, который можно проверить подложным файлом.
const sevenZipGameHex = "377abcaf271c0004ece9cab51e00000000000000ae000000000000003905f0606173736574734d5a2d747970686f6e2d72656772657373696f6e2d657865010406000209061800070b02000101000101000c061800080a018e7dd1798d541248000005040e01c019020000116700470061006d0065000000470061006d0065002f0064006100740061000000470061006d0065002f0064006100740061002f0063006f006e00740065006e0074002e00700061006b000000470061006d0065002f00470061006d0065002e006500780065000000190015120100100000001000000020000000200000000000"

func writeSevenZipGame(t *testing.T, path string) {
	t.Helper()
	raw, err := hex.DecodeString(sevenZipGameHex)
	if err != nil {
		t.Fatalf("decode 7z fixture: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir for %s: %v", path, err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// blockStore подменяет каталог состояния файлом: MkdirAll в store.save падает
// сразу и на Windows, и на POSIX, а права каталога тут ни при чём
// (requireUnwritableDir на Windows пропускает тест целиком). Подмена самого
// installations.json каталогом не годится: storage.WriteAtomic на Windows считает
// отказ переименования временным и повторяет его больше полусекунды. unblock
// возвращает каталог со всем содержимым, и cleanup делает это сам до остановки сервиса.
func (r *rig) blockStore() (unblock func()) {
	r.t.Helper()
	return blockStoreDir(r.t, r.s.store.dir)
}

// renameWithRetry не сдаётся на первой ошибке: на Windows переименование
// каталога конкурирует с антивирусом и индексатором, которые держат файлы внутри.
func renameWithRetry(from, to string) error {
	var err error
	for range 100 {
		if err = os.Rename(from, to); err == nil {
			return nil
		}
		<-time.After(10 * time.Millisecond)
	}
	return err
}

func blockStoreDir(t *testing.T, dir string) func() {
	t.Helper()
	moved := dir + ".moved"
	if err := renameWithRetry(dir, moved); err != nil {
		t.Fatalf("move %s aside: %v", dir, err)
	}
	if err := os.WriteFile(dir, []byte("not a directory"), 0o600); err != nil {
		t.Fatalf("block %s: %v", dir, err)
	}
	done := false
	unblock := func() {
		if done {
			return
		}
		done = true
		if err := os.Remove(dir); err != nil {
			t.Errorf("unblock %s: %v", dir, err)
			return
		}
		if err := renameWithRetry(moved, dir); err != nil {
			t.Errorf("restore %s: %v", dir, err)
		}
	}
	t.Cleanup(unblock)
	return unblock
}

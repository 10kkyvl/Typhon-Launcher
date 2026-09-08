package wine

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// defaultSharedBottle — имя бутыля, в котором у пользователя стоит windows
// Steam. Общий бутыль нужен потому, что Steam Overlay, Steam API и
// достижения работают только когда игра и сам Steam живут в одном
// wine-префиксе: из соседнего бутыля игра Steam просто не видит.
const defaultSharedBottle = "Steam"

// SharedBottleEnv позволяет назвать бутыль иначе, не пересобирая лаунчер:
// имя пользовательское, и «Steam» — только самое вероятное из них.
const SharedBottleEnv = "TYPHON_STEAM_BOTTLE"

// ErrNoSharedBottle — общего бутыля на машине нет. Не поломка: вызывающий
// обязан откатиться на собственный бутыль игры, а не отказать в запуске.
var ErrNoSharedBottle = errors.New("wine: общий бутыль Steam не найден")

// ErrNoDriveForPath — ни одна буква бутыля не покрывает путь игры.
var ErrNoDriveForPath = errors.New("wine: в бутыле нет буквы, покрывающей путь")

// SharedBottleName отдаёт имя общего бутыля.
func SharedBottleName() string {
	if name := strings.TrimSpace(os.Getenv(SharedBottleEnv)); name != "" {
		return name
	}
	return defaultSharedBottle
}

// SharedBottle отдаёт общий бутыль, нацеленный на конкретную установку.
//
// Бутыль пользовательский: мы его не создаём, не помечаем typhon-bottle.json
// и не удаляем — Typhon в нём гость. Игра при этом остаётся лежать там, где
// лежала: внутрь drive_c ничего не копируется, путь до неё бутыль и так
// видит через одну из своих букв (как минимум через z:, которую CrossOver
// заводит на корень файловой системы).
//
// Key — каталог установки, как и у собственных бутылей: по нему процессы
// этой игры отделяются от Steam и от других игр, живущих в том же бутыле.
func (m *Manager) SharedBottle(destDir string) (Bottle, error) {
	if m.BottlesDir == "" {
		return Bottle{}, ErrNoSharedBottle
	}
	dest, err := filepath.Abs(destDir)
	if err != nil {
		return Bottle{}, fmt.Errorf("путь установки %s: %w", destDir, err)
	}
	name := SharedBottleName()
	path := filepath.Join(m.BottlesDir, name)
	if info, statErr := os.Stat(filepath.Join(path, "dosdevices")); statErr != nil || !info.IsDir() {
		return Bottle{}, fmt.Errorf("%w: %s", ErrNoSharedBottle, path)
	}
	letter, target, ok := drives(path).drive(dest)
	if !ok {
		return Bottle{}, fmt.Errorf("%w: %s в бутыле %s", ErrNoDriveForPath, dest, name)
	}
	return Bottle{Key: dest, Name: name, Path: path, Drive: letter, Games: target, Shared: true}, nil
}

// SharedBottleAny отдаёт общий бутыль без привязки к установке: буквы под
// конкретный путь тут не выбрать, но перевод путей общему бутылю их и не
// требует, а больше от него в этом виде ничего не нужно.
func (m *Manager) SharedBottleAny() (Bottle, bool) {
	path, ok := m.SharedBottlePath()
	if !ok {
		return Bottle{}, false
	}
	return Bottle{Name: SharedBottleName(), Path: path, Shared: true}, true
}

// SharedBottlePath отдаёт каталог общего бутыля, если он на машине есть.
// Нужен тем, кому бутыль интересен целиком, а не под конкретную установку:
// сейвы игр из общего бутыля лежат в его drive_c, и без него библиотека
// искала бы их только в наших собственных.
func (m *Manager) SharedBottlePath() (string, bool) {
	if m.BottlesDir == "" {
		return "", false
	}
	path := filepath.Join(m.BottlesDir, SharedBottleName())
	if info, err := os.Stat(filepath.Join(path, "drive_c")); err != nil || !info.IsDir() {
		return "", false
	}
	return path, true
}

// driveMap — буквы бутыля и их native-цели.
type driveMap map[string]string

// drives читает dosdevices бутыля. Ошибки чтения не отличаются от пустой
// таблицы: и то и другое значит «через этот бутыль путь не выражается», а
// вызывающему в обоих случаях делать одно и то же.
func drives(bottlePath string) driveMap {
	dosdevices := filepath.Join(bottlePath, "dosdevices")
	entries, err := os.ReadDir(dosdevices)
	if err != nil {
		return nil
	}
	out := make(driveMap, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		// Ровно «x:». Записи вида «x::» — это устройства (/dev/rdisk…), а не
		// каталоги, и путей через них не бывает.
		if len(name) != 2 || name[1] != ':' {
			continue
		}
		target, err := os.Readlink(filepath.Join(dosdevices, name))
		if err != nil {
			continue
		}
		if !filepath.IsAbs(target) {
			// c: указывает на ../drive_c относительно самого dosdevices.
			target = filepath.Join(dosdevices, target)
		}
		out[strings.ToLower(name[:1])] = filepath.Clean(target)
	}
	return out
}

// drive выбирает букву, под которой бутыль видит native-путь. Побеждает
// самая длинная подходящая цель — ровно так же путь переводит сам wine
// (проверено на живом CrossOver: /Users/<имя>/… становится Y:\…, а не Z:\…),
// и только совпав с ним, мы узнаем игру в таблице процессов.
func (d driveMap) drive(native string) (letter, target string, ok bool) {
	for candidate, root := range d {
		if !underKey(native, root) {
			continue
		}
		if !ok || len(root) > len(target) {
			letter, target, ok = candidate, root, true
		}
	}
	return letter, target, ok
}

// toNative переводит windows-путь через таблицу букв бутыля.
func (d driveMap) toNative(win string) (string, bool) {
	if len(win) < 3 || win[1] != ':' {
		return "", false
	}
	root, ok := d[strings.ToLower(win[:1])]
	if !ok {
		return "", false
	}
	rel := strings.ReplaceAll(win[3:], `\`, "/")
	return filepath.Join(root, rel), true
}

// matchLaunched опознаёт процесс по windows-пути, которым мы его сами и
// запускали: перевести его в native нечем — путь может лежать за буквой,
// которую бутыль этой установке не выдавал.
func matchLaunched(entry psEntry, winPaths []string) (Process, bool) {
	for _, win := range winPaths {
		if win != "" && strings.EqualFold(entry.winPath, win) {
			return Process{PID: entry.pid, WinPath: entry.winPath, CreatedAt: entry.createdAt}, true
		}
	}
	return Process{}, false
}

// steamExeCandidates — где CrossOver держит steam.exe после обычной
// установки. 32-битный установщик Steam кладёт себя в Program Files (x86)
// даже в 64-битном бутыле, но встречается и вторая раскладка.
var steamExeCandidates = []string{
	filepath.Join("Program Files (x86)", "Steam", "steam.exe"),
	filepath.Join("Program Files", "Steam", "steam.exe"),
}

// SteamExe ищет steam.exe внутри бутыля и отдаёт его windows-путь.
func (b Bottle) SteamExe() (string, error) {
	for _, rel := range steamExeCandidates {
		if _, err := os.Stat(filepath.Join(b.Path, "drive_c", rel)); err == nil {
			return `C:\` + strings.ReplaceAll(rel, "/", `\`), nil
		}
	}
	return "", fmt.Errorf("steam.exe не найден в бутыле %s", b.Name)
}

// steamProcess опознаёт Steam в таблице процессов. Бутыль по строке ps
// не определить — там только windows-путь, одинаковый во всех префиксах, —
// поэтому запущенный в соседнем бутыле Steam мы посчитаем своим. Цена
// ошибки мала: лишний Steam не поднимется, а игра всё равно стартует.
func steamProcess(winPath string) bool {
	return strings.HasSuffix(strings.ToLower(winPath), `\steam.exe`)
}

// SteamRunning отвечает, крутится ли уже windows Steam.
func (m *Manager) SteamRunning() (bool, error) {
	out, err := m.processList()
	if err != nil {
		return false, err
	}
	for _, entry := range parsePS(out) {
		if steamProcess(entry.winPath) {
			return true, nil
		}
	}
	return false, nil
}

// steamAppearTimeout — сколько ждём появления процесса Steam. Ждём именно
// появления, а не готовности: клиент прогружается дольше любого разумного
// таймаута, а вызывающий держит мьютекс библиотеки, и висеть там минуту
// нельзя. Игре достаточно, чтобы Steam поднимался параллельно.
const steamAppearTimeout = 20 * time.Second

const steamPollInterval = 500 * time.Millisecond

// EnsureSteam поднимает Steam в бутыле, если его ещё нет. Повторно уже
// запущенный клиент не стартует: второй экземпляр Steam закрывается сам, но
// успевает отобрать фокус у игры.
//
// Возвращает true, если Steam пришлось запускать.
func (m *Manager) EnsureSteam(ctx context.Context, b Bottle) (bool, error) {
	running, err := m.SteamRunning()
	if err != nil {
		return false, err
	}
	if running {
		slog.Info("steam already running", "bottle", b.Name)
		return false, nil
	}
	exe, err := b.SteamExe()
	if err != nil {
		return false, err
	}
	slog.Info("starting steam", "bottle", b.Name, "exe", exe)
	// -silent: клиент нужен игре, а не пользователю, и разворачивать поверх
	// неё окно библиотеки Steam незачем. Окно логина -silent не прячет.
	if err := m.StartDetached(ctx, b, Cmd{Path: exe, Args: []string{"-silent"}}); err != nil {
		return false, fmt.Errorf("запуск Steam в бутыле %s: %w", b.Name, err)
	}
	if err := m.awaitSteam(ctx); err != nil {
		return true, err
	}
	return true, nil
}

func (m *Manager) awaitSteam(ctx context.Context) error {
	ticker := time.NewTicker(steamPollInterval)
	defer ticker.Stop()
	deadline := time.After(steamAppearTimeout)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline:
			// Не ошибка запуска игры: Steam мог просто разворачиваться
			// дольше нашего терпения, и игра со Steam API дождётся его сама.
			slog.Warn("steam did not appear in time", "timeout", steamAppearTimeout)
			return nil
		case <-ticker.C:
		}
		running, err := m.SteamRunning()
		if err != nil {
			return err
		}
		if running {
			return nil
		}
	}
}

// KillProcesses валит процессы одной установки, не трогая остальной бутыль.
// Для общего бутыля это единственный допустимый способ остановки: wineserver
// -k свалил бы вместе с игрой и Steam, и все другие игры в том же префиксе.
func (m *Manager) KillProcesses(ctx context.Context, b Bottle) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return m.killOwned(b)
}

// stopBottle останавливает то, что в бутыле принадлежит этой операции:
// собственный бутыль — целиком, общий — только своими процессами. Без ctx,
// потому что зовётся в том числе по уже отменённому контексту, когда
// перечислять процессы всё равно надо.
//
// winPaths — то, что мы в этом бутыле сами запустили. Каталогом установки
// такой процесс не опознаётся: установщик лежит в папке загрузок, а не в
// том каталоге, куда ставит, и по одному только ключу бутыля оборванная
// разведка оставляла бы его висеть в чужом бутыле навсегда.
func (m *Manager) stopBottle(b Bottle, winPaths ...string) error {
	if b.Shared {
		return m.killOwned(b, winPaths...)
	}
	return m.Kill(b)
}

func (m *Manager) killOwned(b Bottle, winPaths ...string) error {
	out, err := m.processList()
	if err != nil {
		return err
	}
	found := make([]Process, 0, 4)
	for _, entry := range parsePS(out) {
		if p, ok := match(entry, b); ok {
			found = append(found, p)
			continue
		}
		if p, ok := matchLaunched(entry, winPaths); ok {
			found = append(found, p)
		}
	}
	var failed []string
	for _, p := range found {
		proc, err := os.FindProcess(p.PID)
		if err != nil {
			failed = append(failed, fmt.Sprintf("%d: %v", p.PID, err))
			continue
		}
		// SIGKILL, а не Signal(SIGTERM): windows-процесс под wine на
		// вежливое завершение не реагирует, а «Стоп» обязан сработать.
		if err := proc.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
			failed = append(failed, fmt.Sprintf("%d: %v", p.PID, err))
		}
	}
	if len(failed) > 0 {
		return fmt.Errorf("остановка игры в бутыле %s: %s", b.Name, strings.Join(failed, "; "))
	}
	return nil
}

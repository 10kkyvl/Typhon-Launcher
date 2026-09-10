//go:build darwin && !devmock

package install

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"typhon/internal/installguard/asset"
	"typhon/internal/uierr"
	"typhon/internal/wine"
)

var errWineMissing = uierr.New("wine.not_installed", "для установки игр на macOS нужен CrossOver")

// wineRunner ставит игру в её собственном бутыле. Повышения прав здесь нет:
// UAC на macOS не существует; повышение прав через Windows-воркер не
// используется. Wine-помощник управляет деревом процессов и отменой отдельно.
type wineRunner struct {
	gamesPath func() string
	detect    func() (wine.Runtime, error)
	// runCmd подменяется в тестах: настоящий cxstart на машине сборки может
	// отсутствовать, а проверять надо собранную команду.
	runCmd func(ctx context.Context, b wine.Bottle, c wine.Cmd) (int, error)
}

func newRunner(gamesPath func() string) runner {
	return wineRunner{gamesPath: gamesPath, detect: wine.Detect}
}

func (r wineRunner) run(ctx context.Context, spec runSpec) (int, error) {
	bottle, err := r.bottle(spec)
	if err != nil {
		return 0, err
	}

	// Бутыль уже получена строкой выше через Ensure — discoverWithBottle
	// переиспользует именно её вместо attemptDiscovery, которая сама снова
	// сканирует каталог бутылей CrossOver (находка "двойной скан").
	outcome, err := discoverWithBottle(ctx, spec.discovery(), bottle, r.doRun)
	if err != nil {
		return 0, err
	}
	if outcome.reason != "" {
		slog.Warn("component discovery skipped", "path", spec.Path, "reason", outcome.reason)
	}
	prepared, err := applyDiscoveredComponents(spec, outcome.components)
	if err != nil {
		return 0, err
	}
	return r.runPrepared(ctx, prepared, bottle)
}

// bottle заводит бутыль под каталог установки. Для удаления Destination
// пустой, и каталогом установки служит папка самого деинсталлятора.
//
// Общий бутыль предпочтительнее собственного: если установщик пропишет игру
// в его реестр, запуск (wineStarter.bottleFor, internal/library/process_darwin.go)
// найдёт её в том же префиксе, и Steam API, оверлей и достижения будут
// рабочими. Разные бутыли для установки и запуска развели бы VC-редисты,
// ассоциации и запись удаления не туда, куда реально легла игра.
func (r wineRunner) bottle(spec runSpec) (wine.Bottle, error) {
	dest := spec.Destination
	if dest == "" {
		dest = filepath.Dir(spec.Path)
	}
	if dest == "" || dest == "." || dest == string(filepath.Separator) {
		return wine.Bottle{}, errEmptyDestination
	}
	rt, err := r.detect()
	if err != nil {
		return wine.Bottle{}, errWineMissing
	}
	manager := wine.NewManager(rt)

	sharedBottle, sharedErr := manager.SharedBottle(dest)
	if sharedErr == nil {
		slog.Info("installing into shared bottle", "bottle", sharedBottle.Name, "shared", sharedBottle.Shared, "destination", dest)
		return sharedBottle, nil
	}
	if !wine.SharedBottleUnavailable(sharedErr) {
		return wine.Bottle{}, uierr.Wrap("wine.bottle_create_failed", sharedErr)
	}
	// Общего бутыля нет или он не покрывает путь установки — обычное
	// состояние машины без общего Steam, а не поломка.
	slog.Info("shared bottle unavailable, using own bottle", "destination", dest, "error", sharedErr)

	games := ""
	if r.gamesPath != nil {
		games = r.gamesPath()
	}
	bottle, err := manager.Ensure(dest, games)
	if err != nil {
		return wine.Bottle{}, uierr.Wrap("wine.bottle_create_failed", err)
	}
	slog.Info("installing into own bottle", "bottle", bottle.Name, "shared", bottle.Shared, "destination", dest)
	return bottle, nil
}

func (r wineRunner) runPrepared(ctx context.Context, spec runSpec, bottle wine.Bottle) (int, error) {
	winPath, err := bottle.ToWindows(spec.Path)
	if err != nil {
		return 0, fmt.Errorf("путь установщика: %w", err)
	}
	// WaitChildren: установщик распаковывает себя во временный каталог и
	// работает уже оттуда, поэтому ждать надо всё дерево, а не загрузчик.
	args := spec.Args
	if spec.Engine == EngineNsis && spec.Destination != "" {
		args = []string{"/S", "/D=" + spec.Destination}
	}
	args, err = winePathArgs(bottle, args)
	if err != nil {
		return 0, err
	}
	cmd := wine.Cmd{Path: winPath, Args: args, Log: wineInstallerLog(spec.LogPath), WaitChildren: true, InstallerGuard: true, HideProgress: spec.Hidden, Limit32BitAddressSpace: isFitGirlInstaller(spec.Engine, spec.Path), DLLOverrides: wineInstallerDLLOverrides(spec.Engine, spec.Path)}
	if spec.Dir != "" {
		dir, dirErr := bottle.ToWindows(spec.Dir)
		if dirErr != nil {
			return 0, fmt.Errorf("рабочая папка установщика: %w", dirErr)
		}
		cmd.WorkDir = dir
	}
	code, err := r.doRun(ctx, bottle, cmd)
	return code, classifyRunErr(err)
}

func (r wineRunner) doRun(ctx context.Context, bottle wine.Bottle, cmd wine.Cmd) (int, error) {
	if r.runCmd != nil {
		return r.runCmd(ctx, bottle, cmd)
	}
	rt, err := r.detect()
	if err != nil {
		return 0, errWineMissing
	}
	manager := wine.NewManager(rt)
	if !cmd.InstallerGuard {
		return manager.Run(ctx, bottle, cmd)
	}
	// --cx-log otherwise enables expensive per-exception unwind/module traces.
	cmd.DebugMessages = "-all,-seh,-unwind,-process,-module,-loaddll,-threadname,err+all"
	path, cleanup, err := asset.Extract(filepath.Join(bottle.Path, "drive_c"))
	if err != nil {
		return 0, err
	}
	removeHelper := true
	defer func() {
		if removeHelper {
			cleanup()
		}
	}()
	helperPath, err := bottle.ToWindows(path)
	if err != nil {
		return 0, fmt.Errorf("путь помощника установщика: %w", err)
	}
	original := cmd.Path
	if cmd.WorkDir == "" {
		if end := strings.LastIndex(original, `\`); end >= 0 {
			cmd.WorkDir = original[:end]
		}
	}
	mode := "music"
	if cmd.HideProgress {
		mode = "quiet"
	}
	if cmd.Limit32BitAddressSpace {
		mode = "repack-" + mode
	}
	cancelPath := filepath.Join(filepath.Dir(path), "cancel")
	cancelWin, err := bottle.ToWindows(cancelPath)
	if err != nil {
		return 0, err
	}
	cmd.WaitChildren = false // the bridge waits for writers; Wine services may outlive it
	cmd.CancelFile = cancelPath
	cmd.Args = append([]string{mode, cancelWin, "--", original}, cmd.Args...)
	cmd.Path = helperPath
	cmd.StopPaths = []string{original}
	code, err := manager.Run(ctx, bottle, cmd)
	if errors.Is(err, wine.ErrTreeNotStopped) {
		removeHelper = false
	}
	return code, err
}

// classifyRunErr переводит отказ Kill подтвердить остановку бутыля в
// errInstallerNotConfirmedStopped — тот же класс ошибки, что и у повышенного
// воркера на Windows (elevated.go), на который уже реагирует discardSilent:
// без этого RemoveAll на macOS шёл бы по каталогу, в который ещё может
// писать не убитый установщик (инвариант 9).
func classifyRunErr(err error) error {
	if err == nil || !errors.Is(err, wine.ErrTreeNotStopped) {
		return err
	}
	return fmt.Errorf("%w: %w", errInstallerNotConfirmedStopped, err)
}

// Convert path-valued installer options as well as the executable itself.
func winePathArgs(b wine.Bottle, args []string) ([]string, error) {
	out := append([]string(nil), args...)
	for i, arg := range out {
		prefix, value := "", arg
		if key, v, ok := strings.Cut(arg, "="); ok {
			prefix, value = key+"=", v
		}
		if !filepath.IsAbs(value) || (prefix == "" && strings.Count(value, "/") < 2) {
			continue
		}
		win, err := b.ToWindows(value)
		if err != nil {
			return nil, fmt.Errorf("параметр установщика %s: %w", prefix, err)
		}
		out[i] = prefix + win
	}
	return out, nil
}

// Inno and cxstart truncate their logs; they must never share the same file.
func wineInstallerLog(path string) string {
	if path == "" {
		return ""
	}
	return path + ".wine.log"
}

// FitGirl's BASS player uses DirectSound and draws an image button, not a
// checkbox. Disable that optional playback API for this launch only. Keep this
// profile narrow: other installers may require DirectSound for more than music.
func isFitGirlInstaller(engine Engine, installer string) bool {
	if engine != EngineInno || !strings.EqualFold(filepath.Base(installer), "setup.exe") {
		return false
	}
	//nolint:gosec // G703: local user-selected game/helper path; no network path input or privileged filesystem access.
	marker, err := os.Stat(filepath.Join(filepath.Dir(installer), "fg-01.bin"))
	if err != nil || !marker.Mode().IsRegular() {
		return false
	}
	return true
}

func wineInstallerDLLOverrides(engine Engine, installer string) string {
	if isFitGirlInstaller(engine, installer) {
		return "dsound="
	}
	return ""
}

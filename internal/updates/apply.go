package updates

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"typhon/internal/download"
	"typhon/internal/install"
	"typhon/internal/library"
)

const (
	carryOverLimit   = 4 << 30
	copyBufferSize   = 1 << 20
	installWaitSlack = 2 * time.Hour
)

var (
	errDownloadFailed = errors.New("не удалось скачать данные обновления")
	errInstallFailed  = errors.New("не удалось установить обновление")
	errStagingEmpty   = errors.New("временная установка пуста")
	errNoLaunchTarget = errors.New("исполняемый файл не найден после обновления")
	errSwapFailed     = errors.New("не удалось заменить установленную версию")
	errCarryOver      = errors.New("не удалось перенести пользовательские файлы из предыдущей версии")

	errUnavailablePrefetch = errors.New("предварительная загрузка недоступна для этой стратегии")
)

func (s *Service) StartUpdate(gameID string) error {
	current, ok := s.snapshot(gameID)
	if !ok {
		return errNotTracked
	}
	if current.Plan == nil {
		return errNoPlan
	}
	if s.running(gameID) {
		return errGameRunning
	}
	if s.downloads == nil {
		return errNoDownloads
	}
	if s.library == nil {
		return errNoLibrary
	}
	ctx, started := s.beginJob(gameID)
	if !started {
		return errBusy
	}

	plan := *current.Plan
	entry := UpdateHistory{
		ID:            newID(),
		GameID:        gameID,
		FromVersion:   plan.InstalledVersion,
		ToVersion:     plan.TargetVersion,
		Strategy:      string(plan.Strategy),
		DownloadBytes: plan.DownloadBytes,
		StartedAt:     time.Now(),
		Status:        HistoryRunning,
	}
	s.appendHistory(entry)

	snap, _ := s.mutate(gameID, func(u *Update) {
		u.State = StateUpdating
		u.Step = StepDownload
		u.Progress = 0
		u.Error = ""
		u.Message = ""
	})
	emit(eventStarted, snap)
	slog.Info("update started", "game", gameID, "strategy", plan.Strategy,
		"from", plan.InstalledVersion, "to", plan.TargetVersion, "bytes", plan.DownloadBytes)

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		defer s.endJob(gameID)
		err := s.runUpdate(ctx, plan)
		switch {
		case err == nil:
			s.finishHistory(entry.ID, HistoryCompleted, "")
			done, _ := s.mutate(gameID, func(u *Update) {
				u.State = StateIdle
				u.Step = ""
				u.Progress = 1
				u.Plan = nil
				u.Error = ""
				u.Availability = UpdateAvailability{Kind: KindNone, GameID: gameID, InstalledVersion: plan.TargetVersion}
			})
			emit(eventCompleted, done)
			slog.Info("update completed", "game", gameID, "version", plan.TargetVersion)
			s.recheck(gameID)
		case ctx.Err() != nil:
			s.finishHistory(entry.ID, HistoryFailed, interruptedUpdateText)
			s.mutate(gameID, func(u *Update) {
				u.State = StateAvailable
				u.Step = ""
				u.Progress = 0
				u.Error = interruptedUpdateText
			})
		default:
			s.finishHistory(entry.ID, HistoryFailed, err.Error())
			failed, _ := s.mutate(gameID, func(u *Update) {
				u.State = StateFailed
				u.Step = ""
				u.Progress = 0
				u.Error = err.Error()
			})
			emit(eventFailed, failed)
			slog.Error("update failed", "game", gameID, "error", err)
		}
	}()
	return nil
}

func (s *Service) CancelUpdate(gameID string) error {
	current, ok := s.snapshot(gameID)
	if !ok {
		return errNotTracked
	}
	if current.DownloadID != "" && s.downloads != nil {
		if err := s.downloads.Cancel(current.DownloadID); err != nil {
			slog.Warn("cancel update download", "game", gameID, "error", err)
		}
	}
	s.cancelJob(gameID)
	return nil
}

func (s *Service) runUpdate(ctx context.Context, plan UpdatePlan) error {
	handler := s.strategyFor(plan.Strategy)
	if handler == nil {
		return errUpdateFailed
	}
	return handler.Apply(ctx, plan)
}

func (s *Service) setStep(gameID string, step StepKind, message string) {
	s.mutate(gameID, func(u *Update) {
		u.Step = step
		u.Message = message
	})
}

func (s *Service) downloadRelease(ctx context.Context, plan UpdatePlan, releaseID, destination string, inPlace, flat bool) (download.Download, error) {
	if s.releases == nil {
		return download.Download{}, errNoTarget
	}
	release, ok := s.releases.FindRelease(releaseID)
	if !ok || len(release.URIs) == 0 {
		return download.Download{}, errNoTarget
	}
	if existing, found := s.existingTask(plan.GameID, releaseID); found {
		s.mutate(plan.GameID, func(u *Update) { u.DownloadID = existing.ID })
		return existing, nil
	}
	task, err := s.downloads.AddTask(ctx, download.AddRequest{
		Source:      release.URIs[0],
		InfoHash:    release.InfoHash,
		Destination: destination,
		Name:        release.RawTitle,
		Flat:        flat,
		InPlace:     inPlace,
		Verify:      inPlace,
		Origin: download.Origin{
			ReleaseID:    release.ID,
			SourceID:     release.SourceID,
			Version:      releaseVersion(release),
			Purpose:      download.PurposeUpdate,
			UpdatePlanID: plan.ID,
			LibraryID:    plan.GameID,
		},
	})
	if err != nil {
		return download.Download{}, err
	}
	s.mutate(plan.GameID, func(u *Update) { u.DownloadID = task.ID })
	return task, nil
}

func (s *Service) existingTask(gameID, releaseID string) (download.Download, bool) {
	for _, task := range s.downloads.ByOrigin(gameID, download.PurposeUpdate) {
		if task.Origin.ReleaseID != releaseID || task.Status == download.StatusFailed {
			continue
		}
		return task, true
	}
	return download.Download{}, false
}

// PrefetchUpdate downloads the update data without touching the installation.
func (s *Service) PrefetchUpdate(gameID string) error {
	current, ok := s.snapshot(gameID)
	if !ok {
		return errNotTracked
	}
	if current.Plan == nil {
		return errNoPlan
	}
	if current.Plan.Strategy == StrategyTorrentReuse {
		return errUnavailablePrefetch
	}
	if s.downloads == nil {
		return errNoDownloads
	}
	ctx, started := s.beginJob(gameID)
	if !started {
		return errBusy
	}
	plan := *current.Plan
	s.mutate(gameID, func(u *Update) {
		u.State = StateDownloading
		u.Step = StepDownload
		u.Progress = 0
		u.Error = ""
	})

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		defer s.endJob(gameID)
		err := s.prefetch(ctx, plan)
		s.mutate(gameID, func(u *Update) {
			switch {
			case err == nil:
				u.State = StateReady
				u.Step = ""
				u.Progress = 1
			case ctx.Err() != nil:
				u.State = StateAvailable
				u.Step = ""
				u.Progress = 0
			default:
				u.State = StateAvailable
				u.Step = ""
				u.Progress = 0
				u.Error = err.Error()
			}
		})
	}()
	return nil
}

func (s *Service) prefetch(ctx context.Context, plan UpdatePlan) error {
	targets := []string{plan.TargetReleaseID}
	if plan.Strategy == StrategyPatchChain {
		targets = targets[:0]
		for _, patch := range plan.Patches {
			targets = append(targets, patch.ReleaseID)
		}
	}
	for _, releaseID := range targets {
		task, err := s.downloadRelease(ctx, plan, releaseID, s.config().DownloadsPath, false, false)
		if err != nil {
			return err
		}
		if err := s.waitDownload(ctx, plan.GameID, task.ID); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) waitDownload(ctx context.Context, gameID, downloadID string) error {
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
		task, err := s.downloads.Get(downloadID)
		if err != nil {
			return errDownloadFailed
		}
		s.mutate(gameID, func(u *Update) { u.Progress = task.Progress })
		switch task.Status {
		case download.StatusCompleted:
			return nil
		case download.StatusFailed:
			if task.Error != "" {
				return errors.New(task.Error)
			}
			return errDownloadFailed
		}
	}
}

func (s *Service) installInto(ctx context.Context, downloadID, destination string) (install.Installation, error) {
	if s.installs == nil {
		return install.Installation{}, errNoInstaller
	}
	waiter := make(chan install.Installation, 1)
	s.mu.Lock()
	s.waiters[downloadID] = waiter
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.waiters, downloadID)
		s.mu.Unlock()
	}()

	if _, err := s.installs.Start(downloadID, install.StartOptions{
		Destination:  destination,
		Unattended:   true,
		SkipRegister: true,
	}); err != nil {
		return install.Installation{}, err
	}

	select {
	case <-ctx.Done():
		return install.Installation{}, ctx.Err()
	case <-time.After(installWaitSlack):
		return install.Installation{}, errInstallFailed
	case item := <-waiter:
		if item.Status != install.StatusCompleted {
			if item.Error != "" {
				return item, errors.New(item.Error)
			}
			return item, errInstallFailed
		}
		return item, nil
	}
}

func (s *Service) applyFullRelease(ctx context.Context, plan UpdatePlan) error {
	game, ok := s.installedGame(plan.GameID)
	if !ok {
		return errNotTracked
	}
	staging, err := stagingDir(game.InstallDir, plan.GameID)
	if err != nil {
		return err
	}
	removeTree(staging)
	defer removeTree(staging)

	s.setStep(plan.GameID, StepDownload, "Загрузка релиза")
	task, err := s.downloadRelease(ctx, plan, plan.TargetReleaseID, s.config().DownloadsPath, false, false)
	if err != nil {
		return err
	}
	if err := s.waitDownload(ctx, plan.GameID, task.ID); err != nil {
		return err
	}

	s.setStep(plan.GameID, StepInstall, "Установка во временную папку")
	item, err := s.installInto(ctx, task.ID, staging)
	if err != nil {
		return err
	}
	s.mutate(plan.GameID, func(u *Update) { u.InstallID = item.ID })

	s.setStep(plan.GameID, StepVerify, "Проверка установки")
	if empty, err := isEmptyDir(staging); err != nil || empty {
		return errStagingEmpty
	}
	if s.running(plan.GameID) {
		return errGameRunning
	}

	s.setStep(plan.GameID, StepSwap, "Замена текущей версии")
	previous, err := previousDir(game.InstallDir)
	if err != nil {
		return err
	}
	if err := swapDirectories(game.InstallDir, staging, previous); err != nil {
		slog.Error("swap install directory", "game", plan.GameID, "error", err)
		return errSwapFailed
	}

	executable, err := resolveExecutable(ctx, game.InstallDir, relativeExecutable(game.InstallDir, game.Executable), item.Executable, staging)
	if err != nil {
		if restoreErr := restoreDirectories(game.InstallDir, previous); restoreErr != nil {
			slog.Error("restore previous version", "game", plan.GameID, "error", restoreErr)
		}
		return err
	}
	if executable == "" {
		slog.Error("no launch target after update", "game", plan.GameID)
		if err := restoreDirectories(game.InstallDir, previous); err != nil {
			slog.Error("restore previous version", "game", plan.GameID, "error", err)
		}
		return errNoLaunchTarget
	}

	carried, err := carryOverExtras(ctx, previous, game.InstallDir)
	if err != nil {
		slog.Error("carry over user files", "game", plan.GameID, "path", previous, "error", err)
		return fmt.Errorf("%w: предыдущая версия сохранена в %s: %w", errCarryOver, previous, err)
	}
	if carried.skipped > 0 {
		slog.Warn("user files not carried over", "game", plan.GameID, "bytes", carried.skipped, "limit", int64(carryOverLimit))
		s.setStep(plan.GameID, StepCleanup, fmt.Sprintf("Не перенесено %d Б пользовательских файлов: превышен лимит", carried.skipped))
	}
	s.rememberPrevious(game, previous)

	s.setStep(plan.GameID, StepCleanup, "")
	return s.registerVersion(ctx, game, plan, executable, game.InstallDir)
}

func (s *Service) applyTorrentReuse(ctx context.Context, plan UpdatePlan) error {
	game, ok := s.installedGame(plan.GameID)
	if !ok {
		return errNotTracked
	}
	s.setStep(plan.GameID, StepRecheck, "Проверка существующих файлов")
	task, err := s.downloadRelease(ctx, plan, plan.TargetReleaseID, game.InstallDir, true, plan.ReuseFlat)
	if err != nil {
		return err
	}
	s.setStep(plan.GameID, StepDownload, "Загрузка изменившихся данных")
	if err := s.waitDownload(ctx, plan.GameID, task.ID); err != nil {
		return err
	}

	s.setStep(plan.GameID, StepVerify, "Проверка установки")
	executable, err := resolveExecutable(ctx, game.InstallDir, relativeExecutable(game.InstallDir, game.Executable), "", "")
	if err != nil {
		return err
	}
	if executable == "" {
		return errNoLaunchTarget
	}
	return s.registerVersion(ctx, game, plan, executable, game.InstallDir)
}

func (s *Service) applyPatchChain(ctx context.Context, plan UpdatePlan) error {
	game, ok := s.installedGame(plan.GameID)
	if !ok {
		return errNotTracked
	}
	staging, err := stagingDir(game.InstallDir, plan.GameID)
	if err != nil {
		return err
	}
	defer removeTree(staging)

	for _, patch := range plan.Patches {
		if err := ctx.Err(); err != nil {
			return err
		}
		if s.running(plan.GameID) {
			return errGameRunning
		}
		s.setStep(plan.GameID, StepDownload, "Загрузка патча "+patch.FromVersion+" → "+patch.ToVersion)
		task, err := s.downloadRelease(ctx, plan, patch.ReleaseID, s.config().DownloadsPath, false, false)
		if err != nil {
			return err
		}
		if err := s.waitDownload(ctx, plan.GameID, task.ID); err != nil {
			return err
		}

		removeTree(staging)
		s.setStep(plan.GameID, StepExtract, "Распаковка патча "+patch.ToVersion)
		if _, err := s.installInto(ctx, task.ID, staging); err != nil {
			return err
		}

		s.setStep(plan.GameID, StepApplyPatch, "Применение патча "+patch.ToVersion)
		if err := install.MergeDir(ctx, staging, game.InstallDir, nil); err != nil {
			slog.Error("apply patch", "game", plan.GameID, "patch", patch.ID, "error", err)
			return errUpdateFailed
		}
		removeTree(staging)
		slog.Info("patch applied", "game", plan.GameID, "from", patch.FromVersion, "to", patch.ToVersion)
	}

	s.setStep(plan.GameID, StepVerify, "Проверка установки")
	executable, err := resolveExecutable(ctx, game.InstallDir, relativeExecutable(game.InstallDir, game.Executable), "", "")
	if err != nil {
		return err
	}
	if executable == "" {
		return errNoLaunchTarget
	}
	return s.registerVersion(ctx, game, plan, executable, game.InstallDir)
}

func (s *Service) registerVersion(ctx context.Context, game library.Game, plan UpdatePlan, executable, installDir string) error {
	if s.library == nil {
		return errNoLibrary
	}
	sourceID := game.SourceID
	if s.releases != nil {
		if release, ok := s.releases.FindRelease(plan.TargetReleaseID); ok {
			sourceID = release.SourceID
		}
	}
	updated, err := s.library.ApplyInstalledUpdate(library.InstalledUpdate{
		ID:            game.ID,
		Executable:    executable,
		InstallDir:    installDir,
		Version:       plan.TargetVersion,
		VersionSource: string(VersionSourceRelease),
		ReleaseID:     plan.TargetReleaseID,
		SourceID:      sourceID,
	})
	if err != nil {
		return err
	}
	s.store.removeManifest(game.ID)
	s.runManifest(ctx, updated)
	return nil
}

func (s *Service) rememberPrevious(game library.Game, path string) {
	policy := s.config().KeepPreviousVersion
	if policy == "off" {
		removeTree(path)
		return
	}
	entry := &Rollback{
		GameID:     game.ID,
		Path:       path,
		InstallDir: game.InstallDir,
		Executable: game.Executable,
		Version:    game.Version,
		ReleaseID:  game.ReleaseID,
		SourceID:   game.SourceID,
		CreatedAt:  time.Now(),
	}
	if policy == "24h" {
		until := entry.CreatedAt.Add(previousKeepDuration)
		entry.KeepUntil = &until
	} else {
		entry.AwaitLaunch = true
	}

	s.mu.Lock()
	s.rollbacks[game.ID] = entry
	s.persistRollbacksLocked()
	if u, ok := s.updates[game.ID]; ok {
		u.CanRollback = true
	}
	s.persistLocked()
	s.mu.Unlock()
}

func (s *Service) Rollback(gameID string) error {
	s.mu.Lock()
	entry, ok := s.rollbacks[gameID]
	s.mu.Unlock()
	if !ok {
		return errNoRollback
	}
	if s.running(gameID) {
		return errGameRunning
	}
	if stat, err := os.Stat(entry.Path); err != nil || !stat.IsDir() {
		s.forgetPrevious(gameID)
		return errNoRollback
	}

	if entry.InstallDir == "" {
		return errEmptyInstallDir
	}
	replaced := entry.InstallDir + replacedSuffix
	removeTree(replaced)
	if _, err := os.Stat(entry.InstallDir); err == nil {
		if err := os.Rename(entry.InstallDir, replaced); err != nil {
			slog.Error("move failed install aside", "game", gameID, "error", err)
			return errSwapFailed
		}
	}
	if err := os.Rename(entry.Path, entry.InstallDir); err != nil {
		slog.Error("restore previous version", "game", gameID, "error", err)
		if renameErr := os.Rename(replaced, entry.InstallDir); renameErr != nil {
			slog.Error("restore failed install", "game", gameID, "error", renameErr)
		}
		return errSwapFailed
	}
	removeTree(replaced)
	s.store.removeManifest(gameID)

	restored := s.library != nil
	if s.library != nil {
		if _, err := s.library.ApplyInstalledUpdate(library.InstalledUpdate{
			ID:            gameID,
			Executable:    entry.Executable,
			InstallDir:    entry.InstallDir,
			Version:       entry.Version,
			VersionSource: string(VersionSourceRelease),
			ReleaseID:     entry.ReleaseID,
			SourceID:      entry.SourceID,
		}); err != nil {
			slog.Error("register rolled back version", "game", gameID, "error", err)
			restored = false
		}
	}
	s.forgetPrevious(gameID)
	if restored {
		if err := s.BuildManifest(gameID); err != nil {
			slog.Warn("manifest after rollback", "game", gameID, "error", err)
		}
	}

	snap, _ := s.mutate(gameID, func(u *Update) {
		u.State = StateIdle
		u.Step = ""
		u.Progress = 0
		u.Error = ""
		u.Plan = nil
		u.CanRollback = false
	})
	slog.Info("update rolled back", "game", gameID, "version", entry.Version)
	emit(eventRollback, snap)
	s.recheck(gameID)
	return nil
}

func (s *Service) forgetPrevious(gameID string) {
	s.mu.Lock()
	delete(s.rollbacks, gameID)
	s.persistRollbacksLocked()
	if u, ok := s.updates[gameID]; ok {
		u.CanRollback = false
	}
	s.persistLocked()
	s.mu.Unlock()
}

func swapDirectories(current, staging, previous string) error {
	removeTree(previous)
	if _, err := os.Stat(current); err == nil {
		if err := os.Rename(current, previous); err != nil {
			return err
		}
	}
	if err := os.Rename(staging, current); err != nil {
		if restoreErr := os.Rename(previous, current); restoreErr != nil {
			slog.Error("restore install directory", "path", current, "error", restoreErr)
		}
		return err
	}
	return nil
}

func restoreDirectories(current, previous string) error {
	if _, err := os.Stat(previous); err != nil {
		return err
	}
	broken := current + replacedSuffix
	removeTree(broken)
	if _, err := os.Stat(current); err == nil {
		if err := os.Rename(current, broken); err != nil {
			return err
		}
	}
	if err := os.Rename(previous, current); err != nil {
		return err
	}
	removeTree(broken)
	return nil
}

func resolveExecutable(ctx context.Context, installDir, relative, installed, staging string) (string, error) {
	if installDir == "" {
		return "", errEmptyInstallDir
	}
	if relative != "" {
		candidate := filepath.Join(installDir, relative)
		stat, err := os.Stat(candidate)
		switch {
		case err == nil && !stat.IsDir():
			return candidate, nil
		case err != nil && !errors.Is(err, fs.ErrNotExist):
			return "", fmt.Errorf("stat %s: %w", candidate, err)
		}
	}
	if installed != "" && staging != "" {
		rel, err := filepath.Rel(staging, installed)
		if err != nil {
			return "", fmt.Errorf("relative path %s: %w", installed, err)
		}
		candidate := filepath.Join(installDir, rel)
		stat, err := os.Stat(candidate)
		switch {
		case err == nil && !stat.IsDir():
			return candidate, nil
		case err != nil && !errors.Is(err, fs.ErrNotExist):
			return "", fmt.Errorf("stat %s: %w", candidate, err)
		}
	}
	candidates, err := install.FindExecutables(ctx, installDir, filepath.Base(installDir))
	if err != nil {
		return "", err
	}
	if len(candidates) == 0 {
		return "", nil
	}
	return candidates[0].Path, nil
}

// Game binaries and packed data are never carried over: a file the new release
// dropped must stay dropped, otherwise the installation ends up mixing builds.
var engineExtensions = map[string]bool{
	".exe": true, ".dll": true, ".so": true, ".dylib": true, ".sys": true, ".drv": true,
	".pdb": true, ".pak": true, ".pck": true, ".assets": true, ".bundle": true,
	".bank": true, ".arc": true, ".vpk": true, ".rpf": true, ".cab": true, ".msi": true,
	".bin": true, ".dat": true, ".wad": true, ".sga": true, ".big": true, ".unity3d": true,
}

type carryReport struct {
	carried int64
	skipped int64
}

// carryOverExtras keeps user files that the new installation does not provide,
// so configs, saves and mods survive a full replacement. Прервавшийся перенос —
// ошибка: после неё вызывающий обязан оставить .previous нетронутым.
func carryOverExtras(ctx context.Context, previous, current string) (carryReport, error) {
	return carryOverLimited(ctx, previous, current, carryOverLimit)
}

func carryOverLimited(ctx context.Context, previous, current string, limit int64) (carryReport, error) {
	if previous == "" || current == "" {
		return carryReport{}, errEmptyInstallDir
	}
	var report carryReport
	buf := make([]byte, copyBufferSize)
	err := filepath.WalkDir(previous, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(previous, path)
		if err != nil {
			return fmt.Errorf("relative path %s: %w", path, err)
		}
		target := filepath.Join(current, rel)
		if _, err := os.Lstat(target); err == nil {
			return nil
		} else if !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("stat %s: %w", target, err)
		}
		if d.Type()&fs.ModeSymlink != 0 {
			return carrySymlink(path, target)
		}
		if !d.Type().IsRegular() {
			return fmt.Errorf("%s: нерегулярный файл (%s)", rel, d.Type())
		}
		if engineExtensions[strings.ToLower(filepath.Ext(path))] {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return fmt.Errorf("stat %s: %w", path, err)
		}
		if report.carried+info.Size() > limit {
			report.skipped += info.Size()
			slog.Warn("carry over limit reached", "file", rel, "bytes", info.Size(), "limit", limit)
			return nil
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := copyFile(path, target, buf); err != nil {
			return fmt.Errorf("copy %s: %w", rel, err)
		}
		report.carried += info.Size()
		return nil
	})
	if err != nil {
		return report, fmt.Errorf("scan previous install %s: %w", previous, err)
	}
	if report.carried > 0 {
		slog.Info("user files carried over", "bytes", report.carried, "path", current)
	}
	return report, nil
}

func carrySymlink(path, target string) error {
	dest, err := os.Readlink(path)
	if err != nil {
		return fmt.Errorf("readlink %s: %w", path, err)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	if err := os.Symlink(dest, target); err != nil {
		return fmt.Errorf("symlink %s: %w", target, err)
	}
	return nil
}

func copyFile(src, dst string, buf []byte) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() {
		if err := in.Close(); err != nil {
			slog.Warn("close source file", "path", src, "error", err)
		}
	}()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.CopyBuffer(out, in, buf); err != nil {
		return errors.Join(err, out.Close(), removeFailedCopy(dst))
	}
	// .previous удаляется сразу после переноса, поэтому данные должны лежать
	// на диске, а не в кеше записи.
	if err := out.Sync(); err != nil {
		return errors.Join(err, out.Close(), removeFailedCopy(dst))
	}
	return out.Close()
}

func removeFailedCopy(dst string) error {
	if err := os.Remove(dst); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("remove partial copy %s: %w", dst, err)
	}
	return nil
}

func isEmptyDir(path string) (bool, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return true, err
	}
	return len(entries) == 0, nil
}

func removeTree(path string) {
	if path == "" {
		return
	}
	if err := os.RemoveAll(path); err != nil {
		slog.Warn("remove directory", "path", path, "error", err)
	}
}

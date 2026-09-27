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
	"typhon/internal/history"
	"typhon/internal/install"
	"typhon/internal/library"
	"typhon/internal/platform"
	"typhon/internal/settings"
	"typhon/internal/sources"
	"typhon/internal/uierr"
	"typhon/internal/usagestats"
)

const (
	carryOverLimit   = 4 << 30
	copyBufferSize   = 1 << 20
	installWaitSlack = 2 * time.Hour
)

var (
	errDownloadFailed = uierr.New("updates.download_failed", "не удалось скачать данные обновления")
	errInstallFailed  = uierr.New("updates.install_failed", "не удалось установить обновление")
	errStagingEmpty   = uierr.New("updates.staging_empty", "временная установка пуста")
	errNoLaunchTarget = uierr.New("updates.no_launch_target", "исполняемый файл не найден после обновления")
	errSwapFailed     = uierr.New("updates.swap_failed", "не удалось заменить установленную версию")
	errCarryOver      = uierr.New("updates.carry_over_failed", "не удалось перенести пользовательские файлы из предыдущей версии")

	errUnavailablePrefetch = uierr.New("updates.prefetch_unavailable", "предварительная загрузка недоступна для этой стратегии")

	errNoFreeSpaceForBackup = uierr.New("updates.no_free_space_for_backup", "недостаточно места для резервной копии перед обновлением")

	errDownloadStalled = uierr.New("updates.download_stalled", "загрузка остановилась: нет сети или источников, повторите обновление позже")
)

// updateStallTimeout bounds how long an update job waits on a download that
// reports StatusDownloading without making progress. The torrent client keeps
// trying past this point; only the update job gives up, so Busy clears and
// the user is not stuck on a job that will never finish on its own.
var updateStallTimeout = 15 * time.Minute

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
	if err := s.validatePlan(gameID, *current.Plan); err != nil {
		s.endJob(gameID)
		return err
	}

	canonicalID := s.canonicalGameID(gameID)

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
	s.recordUsage(usagestats.Event{
		Type:       usagestats.TypeUpdateStarted,
		Timestamp:  time.Now(),
		Properties: usagestats.Properties{GameID: canonicalID},
	})
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
			s.recordUsage(usagestats.Event{
				Type:      usagestats.TypeUpdateCompleted,
				Timestamp: time.Now(),
				Properties: usagestats.Properties{
					GameID:          canonicalID,
					DurationSeconds: int64(time.Since(entry.StartedAt).Seconds()),
				},
			})
			slog.Info("update completed", "game", gameID, "version", plan.TargetVersion)
			s.recheck(gameID)
			s.recordUpdateHistory(history.Record{
				Kind:        history.KindUpdated,
				GameID:      canonicalID,
				Title:       s.gameTitle(gameID),
				FromVersion: plan.InstalledVersion,
				ToVersion:   plan.TargetVersion,
				RefID:       gameID,
			})
		case ctx.Err() != nil:
			// Отмена через CancelUpdate или остановка сервиса. В UI это не
			// «обновление упало», но терминальное событие всё равно нужно:
			// без него update_started никогда не сойдётся с суммой исходов.
			// Отличается по error_code "cancelled".
			s.recordUsage(usagestats.Event{
				Type:      usagestats.TypeUpdateFailed,
				Timestamp: time.Now(),
				Properties: usagestats.Properties{
					GameID:          canonicalID,
					DurationSeconds: int64(time.Since(entry.StartedAt).Seconds()),
					ErrorCode:       usagestats.Classify(ctx.Err()),
				},
			})
			s.finishHistory(entry.ID, HistoryFailed, interruptedUpdateText)
			s.mutate(gameID, func(u *Update) {
				u.State = StateAvailable
				u.Step = ""
				u.Progress = 0
				u.Error = interruptedUpdateText
			})
			s.recordUpdateHistory(history.Record{
				Kind:        history.KindUpdateFailed,
				GameID:      canonicalID,
				Title:       s.gameTitle(gameID),
				FromVersion: plan.InstalledVersion,
				ToVersion:   plan.TargetVersion,
				Detail:      interruptedUpdateText,
				RefID:       gameID,
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
			s.recordUsage(usagestats.Event{
				Type:      usagestats.TypeUpdateFailed,
				Timestamp: time.Now(),
				Properties: usagestats.Properties{
					GameID:          canonicalID,
					DurationSeconds: int64(time.Since(entry.StartedAt).Seconds()),
					ErrorCode:       usagestats.Classify(err),
				},
			})
			slog.Error("update failed", "game", gameID, "error", err)
			s.recordUpdateHistory(history.Record{
				Kind:        history.KindUpdateFailed,
				GameID:      canonicalID,
				Title:       s.gameTitle(gameID),
				FromVersion: plan.InstalledVersion,
				ToVersion:   plan.TargetVersion,
				Detail:      err.Error(),
				RefID:       gameID,
			})
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
	if err := s.validatePlan(plan.GameID, plan); err != nil {
		return err
	}
	handler := s.strategyFor(plan.Strategy)
	if handler == nil {
		return errUpdateFailed
	}
	if plan.SavesPath != "" {
		s.setStep(plan.GameID, StepBackup, "Снимок сохранений")
	}
	snapshot, err := s.backupSaves(ctx, plan)
	if err != nil {
		return err
	}
	if snapshot != "" {
		slog.Info("saves snapshot taken", "game", plan.GameID, "from", plan.SavesPath, "path", snapshot)
		s.mutate(plan.GameID, func(u *Update) { u.SavesBackup = snapshot })
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
	if !ok || len(release.URIs) == 0 || !releaseBelongsToPlan(plan, release) {
		return download.Download{}, errNoTarget
	}
	if existing, found := s.existingTask(plan, release, destination, inPlace, flat); found {
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
			ReleaseID:         release.ID,
			SourceID:          release.SourceID,
			DistributionID:    release.DistributionID,
			ReleaseUploadedAt: release.UploadedAt,
			GameID:            s.canonicalGameID(plan.GameID),
			Version:           releaseVersion(release),
			Purpose:           download.PurposeUpdate,
			UpdatePlanID:      plan.ID,
			LibraryID:         plan.GameID,
		},
	})
	if err != nil {
		return download.Download{}, err
	}
	s.mutate(plan.GameID, func(u *Update) { u.DownloadID = task.ID })
	return task, nil
}

func (s *Service) existingTask(plan UpdatePlan, release sources.Release, destination string, inPlace, flat bool) (download.Download, bool) {
	for _, task := range s.downloads.ByOrigin(plan.GameID, download.PurposeUpdate) {
		if task.Origin.ReleaseID != release.ID || task.Origin.SourceID != release.SourceID ||
			(task.Origin.DistributionID != "" && task.Origin.DistributionID != release.DistributionID) || task.Origin.LibraryID != plan.GameID ||
			task.Origin.Version != releaseVersion(release) || !sameDownloadDestination(task.Destination, destination) ||
			task.InPlace != inPlace || task.Flat != flat ||
			(task.Origin.ReleaseUploadedAt != nil && !samePlanTime(task.Origin.ReleaseUploadedAt, release.UploadedAt)) ||
			task.Status == download.StatusFailed {
			continue
		}
		if (task.Origin.DistributionID == "" && release.DistributionID != "") ||
			(task.Origin.ReleaseUploadedAt == nil && release.UploadedAt != nil) {
			// Legacy origins lack provenance fields. Reuse only with proof that
			// this is the same payload, not a replaced revision of the same ID.
			if release.InfoHash == "" || task.InfoHash == "" || !strings.EqualFold(task.InfoHash, release.InfoHash) {
				continue
			}
		}
		return task, true
	}
	return download.Download{}, false
}

func sameDownloadDestination(a, b string) bool {
	return a == b || platform.SamePath(a, b)
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
	if err := s.validatePlan(gameID, *current.Plan); err != nil {
		s.endJob(gameID)
		return err
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
	var lastProgress int64
	var lastChange time.Time
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
		case download.StatusDownloading:
			now := time.Now()
			if lastChange.IsZero() || task.Downloaded != lastProgress {
				lastProgress = task.Downloaded
				lastChange = now
				continue
			}
			if now.Sub(lastChange) >= updateStallTimeout {
				return errDownloadStalled
			}
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
	if err := s.swapDirectories(plan.GameID, game.InstallDir, staging, previous, plan.TargetVersion); err != nil {
		slog.Error("swap install directory", "game", plan.GameID, "error", err)
		return errSwapFailed
	}

	executable, err := resolveExecutable(ctx, game.InstallDir, relativeExecutable(game.InstallDir, game.Executable), item.Executable, staging)
	if err != nil {
		s.undoSwapAndClear(plan.GameID, game.InstallDir, previous)
		return err
	}
	if executable == "" {
		slog.Error("no launch target after update", "game", plan.GameID)
		s.undoSwapAndClear(plan.GameID, game.InstallDir, previous)
		return errNoLaunchTarget
	}

	// carryOverExtras failing leaves .previous and the journal alone on
	// purpose: the caller cannot decide here whether to roll back a
	// partially migrated installation, so the journal stays and the next
	// startup finishes the decision (invariant 9).
	carried, err := carryOverExtras(ctx, previous, game.InstallDir)
	if err != nil {
		slog.Error("carry over user files", "game", plan.GameID, "path", previous, "error", err)
		return fmt.Errorf("%w: предыдущая версия сохранена в %s: %w", errCarryOver, previous, err)
	}
	if carried.skipped > 0 {
		slog.Warn("user files not carried over", "game", plan.GameID, "bytes", carried.skipped, "limit", int64(carryOverLimit))
		s.setStep(plan.GameID, StepCleanup, fmt.Sprintf("Не перенесено %d Б пользовательских файлов: превышен лимит", carried.skipped))
	}

	s.setStep(plan.GameID, StepCleanup, "")
	if err := s.registerVersion(ctx, game, plan, executable, game.InstallDir); err != nil {
		return err
	}
	if err := s.registerRollback(game, previous); err != nil {
		return err
	}
	if err := s.clearJournal(plan.GameID); err != nil {
		return err
	}
	s.settlePrevious(game.ID, previous)
	return nil
}

// applyTorrentReuse writes new torrent pieces directly into the live install
// (invariant 15), so it takes a verified full copy of the installation before
// the first byte is written, journaled so a crash mid-download can restore it.
func (s *Service) applyTorrentReuse(ctx context.Context, plan UpdatePlan) error {
	game, ok := s.installedGame(plan.GameID)
	if !ok {
		return errNotTracked
	}
	s.setStep(plan.GameID, StepRecheck, "Проверка существующих файлов")
	previous, err := s.backupInPlace(ctx, plan.GameID, game.InstallDir, plan.TargetVersion)
	if err != nil {
		return err
	}

	task, err := s.downloadRelease(ctx, plan, plan.TargetReleaseID, game.InstallDir, true, plan.ReuseFlat)
	if err != nil {
		s.undoSwapAndClear(plan.GameID, game.InstallDir, previous)
		return err
	}
	s.setStep(plan.GameID, StepDownload, "Загрузка изменившихся данных")
	if err := s.waitDownload(ctx, plan.GameID, task.ID); err != nil {
		if stopErr := s.stopRepairDownload(task.ID); stopErr != nil {
			return errors.Join(err, stopErr)
		}
		s.undoSwapAndClear(plan.GameID, game.InstallDir, previous)
		return err
	}

	s.setStep(plan.GameID, StepVerify, "Проверка установки")
	executable, err := resolveExecutable(ctx, game.InstallDir, relativeExecutable(game.InstallDir, game.Executable), "", "")
	if err != nil {
		s.undoSwapAndClear(plan.GameID, game.InstallDir, previous)
		return err
	}
	if executable == "" {
		s.undoSwapAndClear(plan.GameID, game.InstallDir, previous)
		return errNoLaunchTarget
	}

	if err := s.registerVersion(ctx, game, plan, executable, game.InstallDir); err != nil {
		return err
	}
	if err := s.registerRollback(game, previous); err != nil {
		return err
	}
	if err := s.clearJournal(plan.GameID); err != nil {
		return err
	}
	s.settlePrevious(game.ID, previous)
	return nil
}

// backupInPlace takes a verified full copy of installDir before the caller's
// first destructive write, and only then journals the operation: a crash
// during the copy itself leaves installDir untouched, so nothing needs
// recovering, while a crash after the journal is written is guaranteed a
// complete, hashed backup to restore from (invariant 15).
func (s *Service) backupInPlace(ctx context.Context, gameID, installDir, version string) (string, error) {
	return s.backupInPlaceSuffix(ctx, gameID, installDir, version, "")
}
func (s *Service) backupInPlaceSuffix(ctx context.Context, gameID, installDir, version, suffix string) (string, error) {
	previous, err := copyInstallAsideSuffix(ctx, installDir, suffix)
	if err != nil {
		return "", err
	}
	if err := s.setJournal(SwapJournal{
		GameID:     gameID,
		Kind:       JournalInplace,
		InstallDir: installDir,
		Previous:   previous,
		Version:    version,
		StartedAt:  time.Now(),
	}); err != nil {
		removeTree(previous)
		return "", err
	}
	return previous, nil
}

// copyInstallAside takes the verified full copy every in-place strategy needs
// before its first destructive write. A crash during the copy itself leaves
// installDir untouched, so the copy is safe to redo from scratch on the next
// attempt (invariant 15).
func copyInstallAside(ctx context.Context, installDir string) (string, error) {
	return copyInstallAsideSuffix(ctx, installDir, "")
}
func copyInstallAsideSuffix(ctx context.Context, installDir, suffix string) (string, error) {
	previous, err := previousDir(installDir)
	if err != nil {
		return "", err
	}
	previous += suffix
	total, err := install.DirSize(ctx, installDir)
	if err != nil {
		return "", err
	}
	if err := checkBackupFreeSpace(installDir, total); err != nil {
		return "", err
	}
	removeTree(previous)
	if err := install.CopyDirVerified(ctx, installDir, previous, nil); err != nil {
		removeTree(previous)
		return "", err
	}
	return previous, nil
}

// undoSwapAndClear rolls a swap or in-place write back to previous and only
// then clears the journal, so a crash between the two still leaves the
// journal for ServiceStartup to finish.
func (s *Service) undoSwapAndClear(gameID, installDir, previous string) {
	s.mu.Lock()
	entry := s.journals[gameID]
	var journal SwapJournal
	if entry != nil {
		journal = *entry
	} else {
		journal = SwapJournal{InstallDir: installDir, Previous: previous}
	}
	s.mu.Unlock()
	if err := restoreSwapFiles(journal); err != nil {
		slog.Error("restore previous version", "game", gameID, "error", err)
		return
	}

	if err := s.clearJournal(gameID); err != nil {
		slog.Error("clear swap journal", "game", gameID, "error", err)
	}
}

func checkBackupFreeSpace(path string, needed int64) error {
	if needed < 0 {
		return errNoFreeSpaceForBackup
	}
	info, err := platform.GetStorageInfo(path)
	if err != nil {
		return fmt.Errorf("%w: %w", errNoFreeSpaceForBackup, err)
	}
	//nolint:gosec // G115: needed >= 0 checked above, the int64->uint64 conversion is exact
	if info.FreeBytes < uint64(needed) {
		return errNoFreeSpaceForBackup
	}
	return nil
}

// applyPatchChain commits one patch at a time: each successfully merged patch
// registers its own intermediate version before the next patch starts, so a
// chain interrupted partway through never reports a version it did not fully
// apply (invariant 14), and a retry after a crash resumes from the last
// registered version instead of redoing the whole chain.
//
// Every patch merges into the live installation, so the chain takes the same
// full copy the in-place strategies take (invariant 15). Unlike them it keeps
// it as a rollback entry from the first patch on: a chain that stops halfway
// leaves the game at an intermediate version nobody asked for, and the way
// back to the version the player started with is that copy.
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

	s.setStep(plan.GameID, StepBackup, "Резервная копия установки")
	previous, err := copyInstallAside(ctx, game.InstallDir)
	if err != nil {
		return err
	}
	if err := s.registerRollback(game, previous); err != nil {
		return err
	}

	touched, err := s.runPatchChain(ctx, plan, game, staging)
	if err != nil {
		if !touched {
			s.forgetPrevious(plan.GameID)
			removeTree(previous)
		}
		return err
	}
	s.settlePrevious(plan.GameID, previous)
	return nil
}

// runPatchChain reports whether the installation still differs from the copy
// taken before the chain, so a chain that failed without leaving anything
// behind can drop that copy instead of offering the player a rollback to the
// version they are already on.
func (s *Service) runPatchChain(ctx context.Context, plan UpdatePlan, game library.Game, staging string) (touched bool, err error) {
	backup := game.InstallDir + patchBackupSuffix
	applied := 0
	stopped := Patch{}

	// A chain that dies halfway leaves the game on a version nobody asked
	// for, so the failure has to say which patch it stopped on: the code the
	// interface translates travels inside the message and survives the
	// prefix (invariant 24).
	defer func() {
		if err != nil && stopped.ID != "" {
			err = fmt.Errorf("патч %s → %s: %w", stopped.FromVersion, stopped.ToVersion, err)
		}
	}()

	for _, patch := range plan.Patches {
		stopped = patch
		if err := ctx.Err(); err != nil {
			return applied > 0, err
		}
		if s.running(plan.GameID) {
			return applied > 0, errGameRunning
		}
		s.setStep(plan.GameID, StepDownload, "Загрузка патча "+patch.FromVersion+" → "+patch.ToVersion)
		task, err := s.downloadRelease(ctx, plan, patch.ReleaseID, s.config().DownloadsPath, false, false)
		if err != nil {
			return applied > 0, err
		}
		if err := s.waitDownload(ctx, plan.GameID, task.ID); err != nil {
			return applied > 0, err
		}

		removeTree(staging)
		s.setStep(plan.GameID, StepExtract, "Распаковка патча "+patch.ToVersion)
		if _, err := s.installInto(ctx, task.ID, staging); err != nil {
			return applied > 0, err
		}

		s.setStep(plan.GameID, StepApplyPatch, "Применение патча "+patch.ToVersion)
		removeTree(backup)
		if err := s.setJournal(SwapJournal{
			GameID:     plan.GameID,
			Kind:       JournalPatch,
			InstallDir: game.InstallDir,
			Previous:   backup,
			Version:    patch.ToVersion,
			Patch:      patch.ID,
			StartedAt:  time.Now(),
		}); err != nil {
			return applied > 0, err
		}
		if err := install.MergeDirWithBackup(ctx, staging, game.InstallDir, backup, nil); err != nil {
			slog.Error("apply patch", "game", plan.GameID, "patch", patch.ID, "error", err)
			restored := s.undoPatch(plan.GameID, patch.ID, game.InstallDir, backup)
			return applied > 0 || !restored, errUpdateFailed
		}
		removeTree(staging)

		executable, err := resolveExecutable(ctx, game.InstallDir, relativeExecutable(game.InstallDir, game.Executable), "", "")
		if err != nil {
			restored := s.undoPatch(plan.GameID, patch.ID, game.InstallDir, backup)
			return applied > 0 || !restored, err
		}
		if executable == "" {
			restored := s.undoPatch(plan.GameID, patch.ID, game.InstallDir, backup)
			return applied > 0 || !restored, errNoLaunchTarget
		}

		releaseID := patch.ReleaseID
		releaseUploadedAt := patch.UploadedAt
		if patch.ToVersion == plan.TargetVersion {
			releaseID = plan.TargetReleaseID
			releaseUploadedAt = plan.TargetReleaseUploadedAt
		}
		if err := s.registerVersionAs(ctx, game, patch.ToVersion, releaseID, releaseUploadedAt, executable, game.InstallDir); err != nil {
			return true, err
		}
		updated, ok := s.installedGame(plan.GameID)
		if !ok {
			return true, errNotTracked
		}
		game = updated
		if err := s.clearJournal(plan.GameID); err != nil {
			return true, err
		}
		removeTree(backup)
		applied++
		slog.Info("patch applied", "game", plan.GameID, "from", patch.FromVersion, "to", patch.ToVersion)
	}

	return applied > 0, nil
}

// undoPatch rolls the interrupted patch back and reports whether the
// installation is back to its pre-patch state. A restore that itself failed
// leaves files from the patch behind, and the copy taken before the chain is
// then the only way back.
func (s *Service) undoPatch(gameID, patchID, installDir, backup string) bool {
	if err := install.RestoreMergeBackup(installDir, backup); err != nil {
		slog.Error("restore patch backup", "game", gameID, "patch", patchID, "error", err)
		return false
	}
	if err := s.clearJournal(gameID); err != nil {
		slog.Error("clear patch journal", "game", gameID, "error", err)
	}
	return true
}

func (s *Service) registerVersion(ctx context.Context, game library.Game, plan UpdatePlan, executable, installDir string) error {
	return s.registerVersionAs(ctx, game, plan.TargetVersion, plan.TargetReleaseID, plan.TargetReleaseUploadedAt, executable, installDir)
}

func (s *Service) registerVersionAs(ctx context.Context, game library.Game, version, releaseID string, releaseUploadedAt *time.Time, executable, installDir string) error {
	if s.library == nil {
		return errNoLibrary
	}
	sourceID := game.SourceID
	distributionID := game.DistributionID
	if s.releases != nil {
		if release, ok := s.releases.FindRelease(releaseID); ok {
			sourceID = release.SourceID
			distributionID = release.DistributionID
		}
	}
	updated, err := s.library.ApplyInstalledUpdate(library.InstalledUpdate{
		ID:                game.ID,
		Executable:        executable,
		InstallDir:        installDir,
		Version:           version,
		VersionSource:     string(VersionSourceRelease),
		ReleaseID:         releaseID,
		SourceID:          sourceID,
		DistributionID:    distributionID,
		ReleaseUploadedAt: releaseUploadedAt,
	})
	if err != nil {
		return err
	}
	s.store.removeManifest(game.ID)
	s.runManifest(ctx, updated)
	return nil
}

func (s *Service) rememberPrevious(game library.Game, path string) {
	if s.config().KeepPreviousVersion == settings.KeepPreviousOff {
		removeTree(path)
		return
	}
	if err := s.registerRollback(game, path); err != nil {
		slog.Error("register rollback", "game", game.ID, "error", err)
	}
}

// registerRollback records the rollback entry whatever KeepPreviousVersion
// says. A strategy writing into the live installation needs the copy for the
// whole operation, so the policy decides only what happens to it once the
// operation is over — that is settlePrevious, not this.
func (s *Service) registerRollback(game library.Game, path string) error {
	policy := s.config().KeepPreviousVersion
	entry := &Rollback{
		GameID:            game.ID,
		Path:              path,
		InstallDir:        game.InstallDir,
		Executable:        game.Executable,
		Version:           game.Version,
		ReleaseID:         game.ReleaseID,
		SourceID:          game.SourceID,
		DistributionID:    game.DistributionID,
		ReleaseUploadedAt: game.ReleaseUploadedAt,
		CreatedAt:         time.Now(),
	}
	if policy == settings.KeepPreviousDay {
		until := entry.CreatedAt.Add(previousKeepDuration)
		entry.KeepUntil = &until
	} else {
		entry.AwaitLaunch = true
	}

	s.mu.Lock()
	previous, had := s.rollbacks[game.ID]
	s.rollbacks[game.ID] = entry
	if err := s.persistRollbacksLocked(); err != nil {
		if had {
			s.rollbacks[game.ID] = previous
		} else {
			delete(s.rollbacks, game.ID)
		}
		s.markDegradedLocked(err)
		s.mu.Unlock()
		slog.Error("persist rollbacks", "game", game.ID, "error", err)
		return err
	}
	var beforeUpdate *Update
	if u, ok := s.updates[game.ID]; ok {
		before := *u
		beforeUpdate = &before
		u.CanRollback = true
	}
	if err := s.persistLocked(); err != nil {
		if beforeUpdate != nil {
			*s.updates[game.ID] = *beforeUpdate
		}
		s.markDegradedLocked(err)
		s.mu.Unlock()
		slog.Error("persist update", "game", game.ID, "error", err)
		return err
	}
	s.clearDegradedLocked()
	s.mu.Unlock()
	return nil
}

// settlePrevious applies KeepPreviousVersion to a copy that had to survive
// the whole operation, once that operation is over.
func (s *Service) settlePrevious(gameID, path string) {
	if s.config().KeepPreviousVersion != settings.KeepPreviousOff {
		return
	}
	s.forgetPrevious(gameID)
	removeTree(path)
}

func (s *Service) Rollback(gameID string) error {
	s.mu.Lock()
	entry, ok := s.rollbacks[gameID]
	if !ok {
		s.mu.Unlock()
		return errNoRollback
	}
	if s.closing || s.jobs[gameID] != nil || s.rollbackActive[gameID] || s.journals[gameID] != nil {
		s.mu.Unlock()
		return errBusy
	}
	copyEntry := *entry
	entry = &copyEntry
	if s.rollbackActive == nil {
		s.rollbackActive = map[string]bool{}
	}
	s.rollbackActive[gameID] = true
	s.wg.Add(1)
	s.mu.Unlock()
	defer func() { s.mu.Lock(); delete(s.rollbackActive, gameID); s.mu.Unlock(); s.wg.Done() }()
	if s.running(gameID) {
		return errGameRunning
	}
	if entry.InstallDir == "" {
		return errEmptyInstallDir
	}
	current, found := s.installedGame(gameID)
	if !found || filepath.Clean(current.InstallDir) != filepath.Clean(entry.InstallDir) {
		return fmt.Errorf("%w: installation location changed", errSwapFailed)
	}
	if stat, err := os.Stat(entry.Path); err != nil || !stat.IsDir() {
		return errNoRollback
	}
	j := SwapJournal{GameID: gameID, Kind: JournalRollback, InstallDir: entry.InstallDir, Staging: entry.Path, Previous: entry.InstallDir + replacedSuffix, Rollback: entry, StartedAt: time.Now()}
	if exists(j.Previous) {
		return fmt.Errorf("%w: previous rollback recovery is required", errSwapFailed)
	}
	if err := s.setJournal(j); err != nil {
		return err
	}
	if err := s.finishRollback(j); err != nil {
		return err
	}
	s.store.removeManifest(gameID)
	s.mu.Lock()
	manifestCtx := s.ctx
	s.mu.Unlock()
	if game, ok := s.installedGame(gameID); ok && manifestCtx != nil {
		s.runManifest(manifestCtx, game)
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
	// Откат уже выполнен и зафиксирован. Ошибка журнала не должна превращать
	// успешный откат в неуспешный: о сбое журнала сообщает он сам.
	if s.historyRecorder != nil {
		if err := s.historyRecorder(history.Record{
			Kind:        history.KindRolledBack,
			GameID:      s.canonicalGameID(gameID),
			Title:       s.gameTitle(gameID),
			FromVersion: current.Version,
			ToVersion:   entry.Version,
			RefID:       gameID,
		}); err != nil {
			slog.Error("record history", "kind", history.KindRolledBack, "game", gameID, "error", err)
		}
	}
	return nil
}

func (s *Service) forgetPrevious(gameID string) {
	s.forgetRollbackBestEffort(gameID)
	s.updateFieldsBestEffort(gameID, func(u *Update) { u.CanRollback = false })
}

// swapDirectories journals before any rename and retains an older rollback
// until the new installation and its rollback metadata are committed.
func (s *Service) swapDirectories(gameID, current, staging, previous, version string) error {
	j := SwapJournal{GameID: gameID, Kind: JournalSwap, InstallDir: current, Staging: staging, Previous: previous, Version: version, StartedAt: time.Now()}
	if _, err := os.Stat(previous); err == nil {
		j.RetainedPrevious = previous + ".retained"
		if exists(j.RetainedPrevious) {
			// Older builds cleared the journal before best-effort cleanup.
			// Reclaim an orphan only after reserving it in a cleanup journal;
			// any unfinished transaction prevents this reservation.
			if err := s.setJournal(SwapJournal{GameID: gameID, Kind: JournalCleanup, RetainedPrevious: j.RetainedPrevious}); err != nil {
				return err
			}
			if err := s.clearJournal(gameID); err != nil {
				return err
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := s.setJournal(j); err != nil {
		return err
	}
	undo := func(cause error) error {
		if err := restoreSwapFiles(j); err != nil {
			return errors.Join(cause, err)
		}
		if err := s.clearJournal(gameID); err != nil {
			return errors.Join(cause, err)
		}
		return cause
	}
	if j.RetainedPrevious != "" {
		if err := os.Rename(previous, j.RetainedPrevious); err != nil {
			return undo(err)
		}
	}
	if _, err := os.Stat(current); err == nil {
		if err := os.Rename(current, previous); err != nil {
			return undo(err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return undo(err)
	}
	if err := os.Rename(staging, current); err != nil {
		return undo(err)
	}
	return nil
}

// An older rollback is retained until the replacement has been registered.
// Presence of retained distinguishes the old .previous from the current
// version renamed aside by this transaction.
func restoreSwapFiles(j SwapJournal) error {
	shifted := j.RetainedPrevious != "" && exists(j.RetainedPrevious)
	if j.RetainedPrevious == "" || shifted {
		if exists(j.Previous) {
			if err := restoreDirectories(j.InstallDir, j.Previous); err != nil {
				return err
			}
		} else if !exists(j.InstallDir) && exists(j.Staging) {
			if err := os.Rename(j.Staging, j.InstallDir); err != nil {
				return err
			}
		}
	}
	if shifted {
		if err := os.Rename(j.RetainedPrevious, j.Previous); err != nil {
			return err
		}
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

const journalMissingRenameText = "Обновление прервано: файлы новой версии на месте, повторите обновление, чтобы зарегистрировать её"

// recoverJournals finishes or rolls back every multi-rename operation left in
// flight by a crash. It runs once from ServiceStartup, before any new job
// can register another journal for the same game.
func (s *Service) recoverJournals() {
	s.mu.Lock()
	journals := make([]SwapJournal, 0, len(s.journals))
	for _, j := range s.journals {
		journals = append(journals, *j)
	}
	s.mu.Unlock()

	for _, j := range journals {
		if j.Kind == JournalInplace && s.downloads != nil {
			stopped := true
			for _, purpose := range []download.Purpose{download.PurposeRepair, download.PurposeUpdate} {
				for _, task := range s.downloads.ByOrigin(j.GameID, purpose) {
					if task.InPlace {
						if err := s.stopRepairDownload(task.ID); err != nil {
							s.failJournalRecovery(j, err)
							stopped = false
							break
						}
					}
				}
			}
			if !stopped {
				continue
			}
		}
		switch j.Kind {
		case JournalCleanup:
			if err := s.clearJournal(j.GameID); err != nil {
				s.failJournalRecovery(j, err)
			}
		case JournalRollback:
			if err := s.finishRollback(j); err != nil {
				s.failJournalRecovery(j, err)
			}
		case JournalSwap:
			s.recoverSwapJournal(j)
		case JournalPatch:
			s.recoverPatchJournal(j)
		case JournalInplace:
			s.recoverInplaceJournal(j)
		default:
			slog.Error("unknown swap journal kind", "game", j.GameID, "kind", j.Kind)
		}
	}
}

func (s *Service) recoverPatchJournal(j SwapJournal) {
	if err := install.RestoreMergeBackup(j.InstallDir, j.Previous); err != nil {
		s.failJournalRecovery(j, err)
		return
	}
	if err := s.restoreJournalMetadata(j); err != nil {
		s.failJournalRecovery(j, err)
		return
	}
	if err := s.clearJournal(j.GameID); err != nil {
		s.failJournalRecovery(j, err)
		return
	}
	msg := fmt.Sprintf("Обновление прервано на патче до %s; предыдущая версия восстановлена", j.Version)
	s.updateFieldsBestEffort(j.GameID, func(u *Update) {
		u.State = StateFailed
		u.Error = msg
		u.Step = ""
		u.Progress = 0
	})
}

func (s *Service) recoverInplaceJournal(j SwapJournal) {
	if exists(j.Previous) {
		if err := restoreDirectories(j.InstallDir, j.Previous); err != nil {
			s.failJournalRecovery(j, err)
			return
		}
		if !strings.HasSuffix(j.Previous, ".repair") {
			s.forgetRollbackBestEffort(j.GameID)
		}
	}
	if err := s.restoreJournalMetadata(j); err != nil {
		s.failJournalRecovery(j, err)
		return
	}
	if err := s.clearJournal(j.GameID); err != nil {
		s.failJournalRecovery(j, err)
		return
	}
	s.updateFieldsBestEffort(j.GameID, func(u *Update) {
		u.State = StateFailed
		u.Error = interruptedUpdateText
		u.Step = ""
		u.Progress = 0
	})
}

func (s *Service) recoverSwapJournal(j SwapJournal) {
	msg := interruptedUpdateText
	if !exists(j.Previous) && !exists(j.InstallDir) && exists(j.Staging) {
		msg = journalMissingRenameText
	}
	if err := restoreSwapFiles(j); err != nil {
		s.failJournalRecovery(j, err)
		return
	}
	if err := s.restoreSwapRollbackMetadata(j); err != nil {
		s.failJournalRecovery(j, err)
		return
	}

	removeTree(j.Staging)
	if err := s.restoreJournalMetadata(j); err != nil {
		s.failJournalRecovery(j, err)
		return
	}
	if err := s.clearJournal(j.GameID); err != nil {
		s.failJournalRecovery(j, err)
		return
	}
	s.updateFieldsBestEffort(j.GameID, func(u *Update) {
		u.State = StateFailed
		u.Error = msg
		u.Step = ""
		u.Progress = 0
	})
}

// failJournalRecovery logs and surfaces the error without clearing the
// journal: the journal is the only record that this operation is unfinished,
// so a recovery step that itself fails must leave it for the next start.
func (s *Service) failJournalRecovery(j SwapJournal, err error) {
	slog.Error("recover swap journal", "game", j.GameID, "kind", j.Kind, "error", err)
	s.updateFieldsBestEffort(j.GameID, func(u *Update) { u.Error = err.Error() })
}

func exists(path string) bool {
	if path == "" {
		return false
	}
	_, err := os.Stat(path)
	return err == nil
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

// canonicalGameID resolves the catalog id usagestats expects. It returns ""
// when the game is not (or no longer) installed, which usagestats accepts as
// "no game id" rather than rejecting the event.
func (s *Service) canonicalGameID(gameID string) string {
	game, ok := s.installedGame(gameID)
	if !ok {
		return ""
	}
	return game.CanonicalGameID
}

// SetUsageRecorder wires the usage-stats sink. Nil clears it.
//
//wails:ignore
func (s *Service) SetUsageRecorder(rec func(usagestats.Event)) {
	s.mu.Lock()
	s.usageRecorder = rec
	s.mu.Unlock()
}

// recordUsage is nil-safe and never blocks the caller: usagestats.Record
// already validates and enqueues without blocking, so this only forwards.
func (s *Service) recordUsage(ev usagestats.Event) {
	s.mu.Lock()
	rec := s.usageRecorder
	s.mu.Unlock()
	if rec == nil {
		return
	}
	rec(ev)
}

func (s *Service) restoreJournalMetadata(j SwapJournal) error {
	if j.Original == nil || s.library == nil {
		return nil
	}
	_, err := s.library.ApplyInstalledUpdate(*j.Original)
	return err
}

package updates

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"typhon/internal/download"
	"typhon/internal/library"
	"typhon/internal/sources"
	"typhon/internal/usagestats"
)

var errRepairUnavailable = errors.New("восстановление недоступно для этой установки")

func (s *Service) emitVerify(gameID, event string, apply func(*VerifyState)) VerifyState {
	s.mu.Lock()
	state, ok := s.verifications[gameID]
	if !ok {
		state = &VerifyState{GameID: gameID}
		s.verifications[gameID] = state
	}
	apply(state)
	if event != eventVerifyUpdated && event != eventRepairUpdated {
		s.persistVerifyLocked()
	}
	snap := *state
	s.mu.Unlock()
	emit(event, snap)
	return snap
}

func (s *Service) GetVerifyState(gameID string) (VerifyState, error) {
	game, tracked := s.installedGame(gameID)
	s.mu.Lock()
	defer s.mu.Unlock()
	state, ok := s.verifications[gameID]
	if !ok {
		return VerifyState{GameID: gameID, Method: MethodPending}, nil
	}
	if state.Running || state.Repairing {
		return *state, nil
	}
	if !tracked || !state.boundTo(game) {
		return VerifyState{GameID: gameID, Method: MethodPending}, nil
	}
	return *state, nil
}

// torrentVerdict reports whether a reuse report is a statement about the
// installed files. A torrent that carries an installer or an archive describes
// what the install was made from, not what it is, and a mapping that found no
// file on disk describes some other directory entirely.
func torrentVerdict(r download.ReuseReport) bool {
	if !r.Applicable {
		return false
	}
	return r.Layout != download.LayoutArchivePackage && r.Layout != download.LayoutInstallerPackage
}

// recordedMapping returns the layout the payload was actually written with,
// which is only meaningful when the download landed in the install directory
// itself rather than being unpacked into it.
func (s *Service) recordedMapping(game library.Game, release sources.Release) *bool {
	if s.downloads == nil || game.SourceDownloadID == "" {
		return nil
	}
	d, err := s.downloads.Get(game.SourceDownloadID)
	if err != nil {
		return nil
	}
	if !strings.EqualFold(d.InfoHash, release.InfoHash) {
		return nil
	}
	if !strings.EqualFold(filepath.Clean(d.Destination), filepath.Clean(game.InstallDir)) {
		return nil
	}
	flat := d.Flat
	return &flat
}

func (s *Service) torrentIdentity(game library.Game) (sources.Release, bool) {
	if s.releases == nil || game.ReleaseID == "" {
		return sources.Release{}, false
	}
	release, ok := s.releases.FindRelease(game.ReleaseID)
	if !ok || release.InfoHash == "" {
		return sources.Release{}, false
	}
	return release, true
}

// VerifyGame checks the installed files against the torrent the game came from,
// falling back to a stored manifest whenever the torrent describes something
// other than what sits in the install directory.
func (s *Service) VerifyGame(gameID string) error {
	game, ok := s.installedGame(gameID)
	if !ok {
		return errNotTracked
	}
	if game.InstallDir == "" {
		return errEmptyInstallDir
	}
	if s.running(gameID) {
		return errGameRunning
	}
	if stat, err := os.Stat(game.InstallDir); err != nil || !stat.IsDir() {
		return errNoIdentity
	}
	release, hasTorrent := s.torrentIdentity(game)
	manifest, hasManifest, err := s.store.loadManifest(gameID)
	if err != nil {
		return fmt.Errorf("load manifest %s: %w", gameID, err)
	}
	if !hasTorrent && !hasManifest {
		s.emitVerify(gameID, eventVerifyCompleted, func(v *VerifyState) {
			*v = unavailableState(game)
		})
		return errNoIdentity
	}

	ctx, started := s.beginJob(gameID)
	if !started {
		return errBusy
	}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		defer s.endJob(gameID)
		s.verify(ctx, game, release, hasTorrent, manifest, hasManifest)
	}()
	return nil
}

func (s *Service) verify(
	ctx context.Context,
	game library.Game,
	release sources.Release,
	hasTorrent bool,
	manifest FileManifest,
	hasManifest bool,
) {
	started := time.Now()
	method := MethodManifest
	if hasTorrent {
		method = MethodTorrent
	}
	s.emitVerify(game.ID, eventVerifyStarted, func(v *VerifyState) {
		*v = VerifyState{GameID: game.ID, Method: method, Running: true}
	})
	s.recordUsage(usagestats.Event{
		Type:       usagestats.TypeVerifyStarted,
		Timestamp:  time.Now(),
		Properties: usagestats.Properties{GameID: game.CanonicalGameID},
	})
	if hasTorrent && s.downloads != nil {
		report, err := s.inspectInstall(ctx, game, release)
		switch {
		case ctx.Err() != nil:
			s.recordVerifyFailure(game.CanonicalGameID, started, terminalCause(ctx, err))
			s.failVerify(ctx, game.ID, err)
			return
		case err != nil && !hasManifest:
			s.recordVerifyFailure(game.CanonicalGameID, started, err)
			s.failVerify(ctx, game.ID, err)
			return
		case err != nil:
			slog.Warn("torrent verify unavailable", "game", game.ID, "error", err)
		case torrentVerdict(report):
			s.applyReuseReport(game, report, started)
			slog.Info("game verified", "game", game.ID, "matched", report.MatchedBytes,
				"missing", report.MissingBytes, "badPieces", report.BadPieces)
			return
		default:
			slog.Info("torrent does not describe install", "game", game.ID,
				"layout", report.Layout, "present", report.PresentFiles, "files", len(report.Files))
		}
	}
	if hasManifest {
		s.verifyByManifest(ctx, game, manifest, started)
		return
	}
	s.emitVerify(game.ID, eventVerifyCompleted, func(v *VerifyState) {
		*v = unavailableState(game)
	})
	s.recordUsage(usagestats.Event{
		Type:      usagestats.TypeVerifyCompleted,
		Timestamp: time.Now(),
		Properties: usagestats.Properties{
			GameID:          game.CanonicalGameID,
			DurationSeconds: int64(time.Since(started).Seconds()),
		},
	})
}

func unavailableState(game library.Game) VerifyState {
	return VerifyState{
		GameID:     game.ID,
		Method:     MethodUnavailable,
		ReleaseID:  game.ReleaseID,
		Version:    game.Version,
		InstallDir: game.InstallDir,
	}
}

func (s *Service) inspectInstall(
	ctx context.Context,
	game library.Game,
	release sources.Release,
) (download.ReuseReport, error) {
	source := ""
	if len(release.URIs) > 0 {
		source = release.URIs[0]
	}
	last := time.Time{}
	return s.downloads.InspectReuse(ctx, download.ReuseRequest{
		Source:   source,
		InfoHash: release.InfoHash,
		Path:     game.InstallDir,
		Flat:     s.recordedMapping(game, release),
	}, func(p download.VerifyProgress) {
		now := time.Now()
		if now.Sub(last) < verifyEventEvery && p.ProcessedBytes < p.TotalBytes {
			return
		}
		last = now
		s.emitVerify(game.ID, eventVerifyUpdated, func(v *VerifyState) {
			v.Progress = ratio(p.ProcessedBytes, p.TotalBytes)
			v.ProcessedBytes = p.ProcessedBytes
			v.CurrentFile = p.CurrentFile
			v.TotalBytes = p.TotalBytes
		})
	})
}

// terminalCause reports the cause a terminal usage event should carry. A
// cancelled ctx wins over whatever error the aborted step happened to return,
// so cancellation lands as error_code "cancelled" instead of being dropped:
// every *_started needs a terminal event, or started counts stop adding up.
func terminalCause(ctx context.Context, cause error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return cause
}

// recordVerifyFailure emits usagestats.TypeVerifyFailed. Cancellation is
// reported too, with error_code "cancelled", so it stays distinguishable from
// a real failure without leaving verify_started unanswered.
func (s *Service) recordVerifyFailure(canonicalID string, started time.Time, cause error) {
	s.recordUsage(usagestats.Event{
		Type:      usagestats.TypeVerifyFailed,
		Timestamp: time.Now(),
		Properties: usagestats.Properties{
			GameID:          canonicalID,
			DurationSeconds: int64(time.Since(started).Seconds()),
			ErrorCode:       usagestats.Classify(cause),
		},
	})
}

func (s *Service) failVerify(ctx context.Context, gameID string, cause error) {
	s.emitVerify(gameID, eventVerifyCompleted, func(v *VerifyState) {
		v.Running = false
		v.Progress = 0
		v.Repairable = false
		v.CheckedAt = nil
		if ctx.Err() == nil && cause != nil {
			v.Error = cause.Error()
		}
	})
}

func (s *Service) applyReuseReport(game library.Game, report download.ReuseReport, started time.Time) {
	now := time.Now()
	damaged := report.MissingFiles > 0 || report.BadPieces > 0
	s.emitVerify(game.ID, eventVerifyCompleted, func(v *VerifyState) {
		*v = VerifyState{
			GameID:          game.ID,
			Method:          MethodTorrent,
			Progress:        1,
			ProcessedBytes:  report.TotalBytes,
			Ratio:           ratio(report.MatchedBytes, report.TotalBytes),
			TotalBytes:      report.TotalBytes,
			OkBytes:         report.MatchedBytes,
			MissingFiles:    report.MissingFiles,
			CorruptedPieces: report.BadPieces,
			Repairable:      damaged,
			Flat:            report.Flat,
			Layout:          string(report.Layout),
			InfoHash:        report.InfoHash,
			ReleaseID:       game.ReleaseID,
			Version:         game.Version,
			InstallDir:      game.InstallDir,
			CheckedAt:       &now,
		}
	})
	s.recordUsage(usagestats.Event{
		Type:      usagestats.TypeVerifyCompleted,
		Timestamp: now,
		Properties: usagestats.Properties{
			GameID:          game.CanonicalGameID,
			DurationSeconds: int64(now.Sub(started).Seconds()),
		},
	})
}

func (s *Service) verifyByManifest(ctx context.Context, game library.Game, manifest FileManifest, started time.Time) {
	s.emitVerify(game.ID, eventVerifyUpdated, func(v *VerifyState) {
		v.Method = MethodManifest
		v.Running = true
		v.Progress = 0
		v.CurrentFile = ""
		v.Issues = nil
		v.Extra = nil
		v.Error = ""
	})
	last := time.Time{}
	result, err := VerifyManifest(ctx, game.InstallDir, manifest, func(p Progress) {
		now := time.Now()
		if now.Sub(last) < verifyEventEvery && p.ProcessedBytes < p.TotalBytes {
			return
		}
		last = now
		s.emitVerify(game.ID, eventVerifyUpdated, func(v *VerifyState) {
			v.Progress = ratio(p.ProcessedBytes, p.TotalBytes)
			v.ProcessedBytes = p.ProcessedBytes
			v.CurrentFile = p.CurrentFile
			v.TotalBytes = p.TotalBytes
		})
	})
	if err != nil {
		s.recordVerifyFailure(game.CanonicalGameID, started, terminalCause(ctx, err))
		s.failVerify(ctx, game.ID, err)
		return
	}
	now := time.Now()
	s.emitVerify(game.ID, eventVerifyCompleted, func(v *VerifyState) {
		*v = VerifyState{
			GameID:          game.ID,
			Method:          MethodManifest,
			Progress:        1,
			ProcessedBytes:  result.TotalBytes,
			Ratio:           result.Ratio(),
			TotalBytes:      result.TotalBytes,
			OkBytes:         result.OkBytes,
			MissingFiles:    result.Count(IssueMissing),
			CorruptedPieces: result.Count(IssueCorrupted, IssueSize),
			UnreadableFiles: result.Count(IssueUnreadable),
			Issues:          result.Issues,
			Extra:           result.Extra,
			ReleaseID:       game.ReleaseID,
			Version:         game.Version,
			InstallDir:      game.InstallDir,
			CheckedAt:       &now,
		}
	})
	s.recordUsage(usagestats.Event{
		Type:      usagestats.TypeVerifyCompleted,
		Timestamp: now,
		Properties: usagestats.Properties{
			GameID:          game.CanonicalGameID,
			DurationSeconds: int64(now.Sub(started).Seconds()),
		},
	})
	slog.Info("game verified against manifest", "game", game.ID,
		"ok", result.OkFiles, "issues", len(result.Issues), "extra", len(result.Extra))
}

// RepairGame downloads only the pieces that no longer match the release torrent.
func (s *Service) RepairGame(gameID string) error {
	game, ok := s.installedGame(gameID)
	if !ok {
		return errNotTracked
	}
	if s.running(gameID) {
		return errGameRunning
	}
	if s.downloads == nil {
		return errNoDownloads
	}
	release, hasTorrent := s.torrentIdentity(game)
	if !hasTorrent {
		return errRepairUnavailable
	}
	state, err := s.GetVerifyState(gameID)
	if err != nil {
		return err
	}
	if state.Method != MethodTorrent || !state.Repairable {
		return errRepairUnavailable
	}
	if !strings.EqualFold(state.InfoHash, release.InfoHash) {
		return errRepairUnavailable
	}
	if state.Layout == string(download.LayoutArchivePackage) || state.Layout == string(download.LayoutInstallerPackage) {
		return errRepairUnavailable
	}

	ctx, started := s.beginJob(gameID)
	if !started {
		return errBusy
	}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		defer s.endJob(gameID)
		s.repair(ctx, game, release, state.Flat)
	}()
	return nil
}

// repair writes corrected torrent pieces directly into the live install, the
// same as applyTorrentReuse, so it takes the same journaled full backup
// first (invariant 15). Unlike an update, a successful repair does not
// change the installed version, so the backup is only a crash safety net:
// on success it is discarded rather than remembered as a rollback target,
// which would otherwise silently replace a real prior-version rollback the
// user could still want.
func (s *Service) repair(ctx context.Context, game library.Game, release sources.Release, flat bool) {
	started := time.Now()
	s.emitVerify(game.ID, eventRepairStarted, func(v *VerifyState) {
		v.Repairing = true
		v.Progress = 0
		v.Error = ""
	})
	s.recordUsage(usagestats.Event{
		Type:       usagestats.TypeRepairStarted,
		Timestamp:  time.Now(),
		Properties: usagestats.Properties{GameID: game.CanonicalGameID},
	})

	previous, err := s.backupInPlace(ctx, game.ID, game.InstallDir, game.Version)
	if err != nil {
		s.recordRepairFailure(game.CanonicalGameID, started, terminalCause(ctx, err))
		s.failRepair(ctx, game.ID, err)
		return
	}

	source := ""
	if len(release.URIs) > 0 {
		source = release.URIs[0]
	}
	task, err := s.downloads.AddTask(ctx, download.AddRequest{
		Source:      source,
		InfoHash:    release.InfoHash,
		Destination: game.InstallDir,
		Name:        game.Title,
		Flat:        flat,
		InPlace:     true,
		Verify:      true,
		Origin: download.Origin{
			ReleaseID: release.ID,
			SourceID:  release.SourceID,
			GameID:    game.CanonicalGameID,
			Version:   releaseVersion(release),
			Purpose:   download.PurposeRepair,
			LibraryID: game.ID,
		},
	})
	if err != nil {
		s.undoSwapAndClear(game.ID, game.InstallDir, previous)
		s.recordRepairFailure(game.CanonicalGameID, started, terminalCause(ctx, err))
		s.failRepair(ctx, game.ID, err)
		return
	}

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			s.undoSwapAndClear(game.ID, game.InstallDir, previous)
			s.recordRepairFailure(game.CanonicalGameID, started, ctx.Err())
			s.failRepair(ctx, game.ID, ctx.Err())
			return
		case <-ticker.C:
		}
		current, err := s.downloads.Get(task.ID)
		if err != nil {
			s.undoSwapAndClear(game.ID, game.InstallDir, previous)
			s.recordRepairFailure(game.CanonicalGameID, started, terminalCause(ctx, errDownloadFailed))
			s.failRepair(ctx, game.ID, errDownloadFailed)
			return
		}
		s.emitVerify(game.ID, eventRepairUpdated, func(v *VerifyState) {
			v.Progress = current.Progress
			v.CurrentFile = ""
		})
		switch current.Status {
		case download.StatusCompleted:
			removeTree(previous)
			if err := s.clearJournal(game.ID); err != nil {
				slog.Error("clear repair journal", "game", game.ID, "error", err)
			}
			now := time.Now()
			s.emitVerify(game.ID, eventRepairCompleted, func(v *VerifyState) {
				v.Repairing = false
				v.Progress = 1
				v.Ratio = 1
				v.OkBytes = v.TotalBytes
				v.MissingFiles = 0
				v.CorruptedPieces = 0
				v.Issues = nil
				v.Error = ""
				v.CheckedAt = &now
			})
			s.recordUsage(usagestats.Event{
				Type:      usagestats.TypeRepairCompleted,
				Timestamp: now,
				Properties: usagestats.Properties{
					GameID:          game.CanonicalGameID,
					DurationSeconds: int64(now.Sub(started).Seconds()),
				},
			})
			slog.Info("game repaired", "game", game.ID, "release", release.ID)
			return
		case download.StatusFailed:
			s.undoSwapAndClear(game.ID, game.InstallDir, previous)
			cause := errors.New(current.Error)
			s.recordRepairFailure(game.CanonicalGameID, started, terminalCause(ctx, cause))
			s.failRepair(ctx, game.ID, cause)
			return
		}
	}
}

// recordRepairFailure emits usagestats.TypeRepairFailed. Cancellation is
// reported too, with error_code "cancelled", so it stays distinguishable from
// a real failure without leaving repair_started unanswered.
func (s *Service) recordRepairFailure(canonicalID string, started time.Time, cause error) {
	s.recordUsage(usagestats.Event{
		Type:      usagestats.TypeRepairFailed,
		Timestamp: time.Now(),
		Properties: usagestats.Properties{
			GameID:          canonicalID,
			DurationSeconds: int64(time.Since(started).Seconds()),
			ErrorCode:       usagestats.Classify(cause),
		},
	})
}

func (s *Service) failRepair(ctx context.Context, gameID string, cause error) {
	s.emitVerify(gameID, eventRepairCompleted, func(v *VerifyState) {
		v.Repairing = false
		v.Progress = 0
		if ctx.Err() == nil && cause != nil {
			v.Error = cause.Error()
		}
	})
}

// BuildManifest records hashes of a controlled installation so that it can be
// verified later even without torrent metadata.
func (s *Service) BuildManifest(gameID string) error {
	game, ok := s.installedGame(gameID)
	if !ok {
		return errNotTracked
	}
	if game.InstallDir == "" {
		return errEmptyInstallDir
	}
	if s.running(gameID) {
		return errGameRunning
	}
	ctx, started := s.beginJob(gameID)
	if !started {
		return errBusy
	}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		defer s.endJob(gameID)
		s.runManifest(ctx, game)
	}()
	return nil
}

// runManifest hashes an installation and stores the result. A failure leaves
// the game without a manifest and says so in the verify state: the caller keeps
// whatever it was doing, but nothing may claim the installation is described.
func (s *Service) runManifest(ctx context.Context, game library.Game) {
	if game.InstallDir == "" {
		s.failVerify(ctx, game.ID, errEmptyInstallDir)
		return
	}
	s.emitVerify(game.ID, eventVerifyStarted, func(v *VerifyState) {
		*v = VerifyState{GameID: game.ID, Method: MethodManifest, Running: true}
	})
	last := time.Time{}
	manifest, err := BuildManifest(ctx, game.InstallDir, func(p Progress) {
		now := time.Now()
		if now.Sub(last) < verifyEventEvery && p.ProcessedBytes < p.TotalBytes {
			return
		}
		last = now
		s.emitVerify(game.ID, eventVerifyUpdated, func(v *VerifyState) {
			v.Progress = ratio(p.ProcessedBytes, p.TotalBytes)
			v.CurrentFile = p.CurrentFile
			v.TotalBytes = p.TotalBytes
		})
	})
	if err != nil {
		s.failVerify(ctx, game.ID, err)
		return
	}
	manifest.GameID = game.ID
	manifest.ReleaseID = game.ReleaseID
	manifest.Version = game.Version
	if err := s.store.saveManifest(manifest); err != nil {
		s.failVerify(ctx, game.ID, fmt.Errorf("сохранение манифеста: %w", err))
		return
	}
	now := time.Now()
	s.emitVerify(game.ID, eventVerifyCompleted, func(v *VerifyState) {
		*v = VerifyState{
			GameID:         game.ID,
			Method:         MethodManifest,
			Progress:       1,
			ProcessedBytes: manifest.TotalSize,
			Ratio:          1,
			TotalBytes:     manifest.TotalSize,
			OkBytes:        manifest.TotalSize,
			ReleaseID:      game.ReleaseID,
			Version:        game.Version,
			InstallDir:     game.InstallDir,
			CheckedAt:      &now,
		}
	})
	slog.Info("manifest built", "game", game.ID, "files", len(manifest.Entries))
}

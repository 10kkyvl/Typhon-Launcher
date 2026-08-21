package updates

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"time"

	"typhon/internal/download"
	"typhon/internal/library"
	"typhon/internal/sources"
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
	s.persistVerifyLocked()
	snap := *state
	s.mu.Unlock()
	emit(event, snap)
	return snap
}

func (s *Service) GetVerifyState(gameID string) (VerifyState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, ok := s.verifications[gameID]
	if !ok {
		return VerifyState{GameID: gameID, Method: MethodUnavailable}, nil
	}
	return *state, nil
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
// or against a stored manifest when no torrent identity is known.
func (s *Service) VerifyGame(gameID string) error {
	game, ok := s.installedGame(gameID)
	if !ok {
		return errNotTracked
	}
	if stat, err := os.Stat(game.InstallDir); err != nil || !stat.IsDir() {
		return errNoIdentity
	}
	release, hasTorrent := s.torrentIdentity(game)
	manifest, hasManifest := s.store.loadManifest(gameID)
	if !hasTorrent && !hasManifest {
		s.emitVerify(gameID, eventVerifyCompleted, func(v *VerifyState) {
			v.Method = MethodUnavailable
			v.Running = false
			v.Repairable = false
			v.Error = errNoIdentity.Error()
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
		if hasTorrent {
			s.verifyByTorrent(ctx, game, release)
			return
		}
		s.verifyByManifest(ctx, game, manifest)
	}()
	return nil
}

func (s *Service) verifyByTorrent(ctx context.Context, game library.Game, release sources.Release) {
	if s.downloads == nil {
		return
	}
	source := ""
	if len(release.URIs) > 0 {
		source = release.URIs[0]
	}
	s.emitVerify(game.ID, eventVerifyStarted, func(v *VerifyState) {
		v.Method = MethodTorrent
		v.Running = true
		v.Progress = 0
		v.CurrentFile = ""
		v.Issues = nil
		v.Extra = nil
		v.Error = ""
	})
	last := time.Time{}
	report, err := s.downloads.InspectReuse(ctx, download.ReuseRequest{
		Source:   source,
		InfoHash: release.InfoHash,
		Path:     game.InstallDir,
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
	if err != nil {
		s.emitVerify(game.ID, eventVerifyCompleted, func(v *VerifyState) {
			v.Running = false
			v.Progress = 0
			v.Repairable = false
			if ctx.Err() == nil {
				v.Error = err.Error()
			}
		})
		return
	}
	s.applyReuseReport(game.ID, report)
	slog.Info("game verified", "game", game.ID, "matched", report.MatchedBytes,
		"missing", report.MissingBytes, "badPieces", report.BadPieces)
}

func (s *Service) verifyByManifest(ctx context.Context, game library.Game, manifest FileManifest) {
	s.emitVerify(game.ID, eventVerifyStarted, func(v *VerifyState) {
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
		s.emitVerify(game.ID, eventVerifyCompleted, func(v *VerifyState) {
			v.Running = false
			v.Progress = 0
			v.Repairable = false
			if ctx.Err() == nil {
				v.Error = err.Error()
			}
		})
		return
	}
	now := time.Now()
	missing := 0
	for _, issue := range result.Issues {
		if issue.Kind == IssueMissing {
			missing++
		}
	}
	s.emitVerify(game.ID, eventVerifyCompleted, func(v *VerifyState) {
		v.Running = false
		v.Progress = 1
		v.CurrentFile = ""
		v.TotalBytes = result.TotalBytes
		v.OkBytes = result.OkBytes
		v.Ratio = result.Ratio()
		v.MissingFiles = missing
		v.CorruptedPieces = len(result.Issues) - missing
		v.Issues = result.Issues
		v.Extra = result.Extra
		v.Repairable = false
		v.Error = ""
		v.CheckedAt = &now
	})
	slog.Info("game verified against manifest", "game", game.ID,
		"ok", result.OkFiles, "issues", len(result.Issues))
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
	state, _ := s.GetVerifyState(gameID)
	if state.Method != MethodTorrent || !state.Repairable {
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

func (s *Service) repair(ctx context.Context, game library.Game, release sources.Release, flat bool) {
	s.emitVerify(game.ID, eventRepairStarted, func(v *VerifyState) {
		v.Repairing = true
		v.Progress = 0
		v.Error = ""
	})
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
			Version:   releaseVersion(release),
			Purpose:   download.PurposeRepair,
			LibraryID: game.ID,
		},
	})
	if err != nil {
		s.failRepair(ctx, game.ID, err)
		return
	}

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			s.failRepair(ctx, game.ID, ctx.Err())
			return
		case <-ticker.C:
		}
		current, err := s.downloads.Get(task.ID)
		if err != nil {
			s.failRepair(ctx, game.ID, errDownloadFailed)
			return
		}
		s.emitVerify(game.ID, eventRepairUpdated, func(v *VerifyState) {
			v.Progress = current.Progress
			v.CurrentFile = ""
		})
		switch current.Status {
		case download.StatusCompleted:
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
			slog.Info("game repaired", "game", game.ID, "release", release.ID)
			return
		case download.StatusFailed:
			s.failRepair(ctx, game.ID, errors.New(current.Error))
			return
		}
	}
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
	ctx, started := s.beginJob(gameID)
	if !started {
		return errBusy
	}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		defer s.endJob(gameID)
		s.emitVerify(gameID, eventVerifyStarted, func(v *VerifyState) {
			v.Method = MethodManifest
			v.Running = true
			v.Progress = 0
			v.Error = ""
		})
		last := time.Time{}
		manifest, err := BuildManifest(ctx, game.InstallDir, func(p Progress) {
			now := time.Now()
			if now.Sub(last) < verifyEventEvery && p.ProcessedBytes < p.TotalBytes {
				return
			}
			last = now
			s.emitVerify(gameID, eventVerifyUpdated, func(v *VerifyState) {
				v.Progress = ratio(p.ProcessedBytes, p.TotalBytes)
				v.CurrentFile = p.CurrentFile
				v.TotalBytes = p.TotalBytes
			})
		})
		if err != nil {
			s.emitVerify(gameID, eventVerifyCompleted, func(v *VerifyState) {
				v.Running = false
				v.Progress = 0
				if ctx.Err() == nil {
					v.Error = err.Error()
				}
			})
			return
		}
		manifest.GameID = gameID
		manifest.ReleaseID = game.ReleaseID
		manifest.Version = game.Version
		if err := s.store.saveManifest(manifest); err != nil {
			slog.Error("save manifest", "game", gameID, "error", err)
		}
		now := time.Now()
		s.emitVerify(gameID, eventVerifyCompleted, func(v *VerifyState) {
			v.Running = false
			v.Progress = 1
			v.CurrentFile = ""
			v.TotalBytes = manifest.TotalSize
			v.OkBytes = manifest.TotalSize
			v.Ratio = 1
			v.Issues = nil
			v.Error = ""
			v.CheckedAt = &now
		})
		slog.Info("manifest built", "game", gameID, "files", len(manifest.Entries))
	}()
	return nil
}

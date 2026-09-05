package updates

import (
	"context"
	"log/slog"
	"os"
	"time"

	"typhon/internal/download"
	"typhon/internal/sources"
	"typhon/internal/uierr"
)

const (
	reuseMinRatio    = 0.15
	stagingOverhead  = 20
	verifyEventEvery = progressPeriodEvents
	planScanText     = "Сверка установленных файлов"
)

var errNoTarget = uierr.New("updates.no_target", "релиз для обновления недоступен")

type planInput struct {
	Installed InstalledGame
	Target    sources.Release
	Patches   []Patch
	Reuse     *download.ReuseReport
}

type strategy interface {
	Type() StrategyType
	CanHandle(ctx context.Context, in planInput) bool
	Plan(ctx context.Context, in planInput) (*UpdatePlan, error)
	Apply(ctx context.Context, plan UpdatePlan) error
}

func (s *Service) strategies() []strategy {
	return []strategy{
		patchChainStrategy{s},
		torrentReuseStrategy{s},
		fullReleaseStrategy{s},
	}
}

func (s *Service) strategyFor(kind StrategyType) strategy {
	for _, candidate := range s.strategies() {
		if candidate.Type() == kind {
			return candidate
		}
	}
	return nil
}

// PreparePlan resolves the concrete update path, including a torrent recheck of
// the existing files when that can cut the download down.
func (s *Service) PreparePlan(gameID string) error {
	current, ok := s.snapshot(gameID)
	if !ok {
		return errNotTracked
	}
	if !current.Availability.Available {
		return errNoTarget
	}
	ctx, started := s.beginJob(gameID)
	if !started {
		return errBusy
	}
	s.mutate(gameID, func(u *Update) {
		u.Planning = true
		u.Error = ""
	})

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		plan, err := s.buildPlan(ctx, gameID)
		s.mutate(gameID, func(u *Update) {
			u.Planning = false
			if err != nil {
				if ctx.Err() == nil {
					u.Error = err.Error()
				}
				return
			}
			u.Plan = plan
			u.Error = ""
			u.Availability.Strategy = plan.Strategy
			u.Availability.EstimatedDownloadBytes = plan.DownloadBytes
			u.Availability.RequiresFullInstall = plan.Strategy == StrategyFullRelease
		})
		prefetch := err == nil && ctx.Err() == nil &&
			s.config().UpdateAutoDownload && plan.Strategy != StrategyTorrentReuse
		s.endJob(gameID)
		if prefetch {
			if err := s.PrefetchUpdate(gameID); err != nil {
				slog.Warn("prefetch update", "game", gameID, "error", err)
			}
		}
	}()
	return nil
}

func (s *Service) planInputFor(ctx context.Context, gameID string) (planInput, error) {
	current, ok := s.snapshot(gameID)
	if !ok {
		return planInput{}, errNotTracked
	}
	game, ok := s.installedGame(gameID)
	if !ok {
		return planInput{}, errNotTracked
	}
	if s.releases == nil {
		return planInput{}, errNoTarget
	}
	target, ok := s.releases.FindRelease(current.Availability.TargetReleaseID)
	if !ok {
		return planInput{}, errNoTarget
	}
	list := s.releases.ReleasesFor(game.CanonicalGameID, game.Title)
	return planInput{
		Installed: installedOf(game),
		Target:    target,
		Patches:   PatchesFrom(list),
	}, ctx.Err()
}

func (s *Service) buildPlan(ctx context.Context, gameID string) (*UpdatePlan, error) {
	in, err := s.planInputFor(ctx, gameID)
	if err != nil {
		return nil, err
	}
	if s.config().AllowTorrentReuse && in.Target.InfoHash != "" {
		if report := s.inspectReuse(ctx, in); report != nil {
			in.Reuse = report
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	for _, candidate := range s.strategies() {
		if !candidate.CanHandle(ctx, in) {
			continue
		}
		plan, err := candidate.Plan(ctx, in)
		if err != nil {
			return nil, err
		}
		plan.ID = newID()
		plan.GameID = gameID
		plan.CreatedAt = time.Now()
		return plan, nil
	}
	return nil, errNoTarget
}

func (s *Service) inspectReuse(ctx context.Context, in planInput) *download.ReuseReport {
	if s.downloads == nil || in.Installed.InstallDir == "" {
		return nil
	}
	if stat, err := os.Stat(in.Installed.InstallDir); err != nil || !stat.IsDir() {
		return nil
	}
	source := ""
	if len(in.Target.URIs) > 0 {
		source = in.Target.URIs[0]
	}
	gameID := in.Installed.GameID
	last := time.Time{}
	report, err := s.downloads.InspectReuse(ctx, download.ReuseRequest{
		Source:   source,
		InfoHash: in.Target.InfoHash,
		Path:     in.Installed.InstallDir,
	}, func(p download.VerifyProgress) {
		now := time.Now()
		if now.Sub(last) < verifyEventEvery && p.ProcessedBytes < p.TotalBytes {
			return
		}
		last = now
		s.planProgress(gameID, func(u *Update) {
			u.Progress = ratio(p.ProcessedBytes, p.TotalBytes)
			u.Message = planScanText
		})
	})
	s.planProgress(gameID, func(u *Update) {
		u.Progress = 0
		u.Message = ""
	})
	if err != nil {
		slog.Warn("inspect reuse", "game", gameID, "error", err)
		return nil
	}
	return &report
}

func ratio(done, total int64) float64 {
	if total <= 0 {
		return 0
	}
	if done >= total {
		return 1
	}
	return float64(done) / float64(total)
}

func baseConfidenceOf(in planInput) float64 {
	compat := Compatible(in.Installed, in.Target)
	match := in.Target.MatchConfidence
	if match <= 0 {
		match = 0.5
	}
	return clamp(versionConfidence(in.Installed) * match * clamp(compat.Confidence))
}

type fullReleaseStrategy struct{ service *Service }

func (fullReleaseStrategy) Type() StrategyType { return StrategyFullRelease }

func (fullReleaseStrategy) CanHandle(_ context.Context, in planInput) bool {
	return len(in.Target.URIs) > 0
}

func (f fullReleaseStrategy) Plan(_ context.Context, in planInput) (*UpdatePlan, error) {
	size := in.Target.Size
	plan := &UpdatePlan{
		InstalledReleaseID: in.Installed.ReleaseID,
		TargetReleaseID:    in.Target.ID,
		InstalledVersion:   in.Installed.Version,
		TargetVersion:      releaseVersion(in.Target),
		Strategy:           StrategyFullRelease,
		DownloadBytes:      size,
		RequiredDiskBytes:  size*2 + size/stagingOverhead,
		BackupRecommended:  true,
		Confidence:         baseConfidenceOf(in),
		RollbackAvailable:  true,
		Steps: []UpdateStep{
			{Kind: StepDownload, Label: "Загрузка релиза", ReleaseID: in.Target.ID, Bytes: size},
			{Kind: StepInstall, Label: "Установка во временную папку"},
			{Kind: StepVerify, Label: "Проверка установки"},
			{Kind: StepSwap, Label: "Замена текущей версии"},
			{Kind: StepCleanup, Label: "Очистка"},
		},
	}
	return plan, nil
}

func (f fullReleaseStrategy) Apply(ctx context.Context, plan UpdatePlan) error {
	return f.service.applyFullRelease(ctx, plan)
}

type torrentReuseStrategy struct{ service *Service }

func (torrentReuseStrategy) Type() StrategyType { return StrategyTorrentReuse }

func (torrentReuseStrategy) CanHandle(_ context.Context, in planInput) bool {
	if in.Reuse == nil || in.Target.InfoHash == "" {
		return false
	}
	if in.Reuse.Layout != download.LayoutDirectFiles {
		return false
	}
	return ratio(in.Reuse.MatchedBytes, in.Reuse.TotalBytes) >= reuseMinRatio
}

func (t torrentReuseStrategy) Plan(_ context.Context, in planInput) (*UpdatePlan, error) {
	report := in.Reuse
	plan := &UpdatePlan{
		InstalledReleaseID: in.Installed.ReleaseID,
		TargetReleaseID:    in.Target.ID,
		InstalledVersion:   in.Installed.Version,
		TargetVersion:      releaseVersion(in.Target),
		Strategy:           StrategyTorrentReuse,
		ReuseFlat:          report.Flat,
		DownloadBytes:      report.MissingBytes,
		ReusedBytes:        report.MatchedBytes,
		RequiredDiskBytes:  report.MissingBytes,
		BackupRecommended:  true,
		Confidence:         baseConfidenceOf(in),
		Steps: []UpdateStep{
			{Kind: StepRecheck, Label: "Проверка существующих файлов", Bytes: report.MatchedBytes},
			{Kind: StepDownload, Label: "Загрузка изменившихся данных", ReleaseID: in.Target.ID, Bytes: report.MissingBytes},
			{Kind: StepVerify, Label: "Проверка установки"},
		},
	}
	return plan, nil
}

func (t torrentReuseStrategy) Apply(ctx context.Context, plan UpdatePlan) error {
	return t.service.applyTorrentReuse(ctx, plan)
}

type patchChainStrategy struct{ service *Service }

func (patchChainStrategy) Type() StrategyType { return StrategyPatchChain }

func (p patchChainStrategy) CanHandle(_ context.Context, in planInput) bool {
	path, ok := FindPatchPath(in.Patches, in.Installed.Version, releaseVersion(in.Target))
	if !ok {
		return false
	}
	if in.Target.Size > 0 && path.Bytes >= in.Target.Size {
		return false
	}
	return true
}

func (p patchChainStrategy) Plan(_ context.Context, in planInput) (*UpdatePlan, error) {
	path, ok := FindPatchPath(in.Patches, in.Installed.Version, releaseVersion(in.Target))
	if !ok {
		return nil, errNoTarget
	}
	var largest int64
	steps := make([]UpdateStep, 0, len(path.Steps)*2+1)
	for _, patch := range path.Steps {
		if patch.Size > largest {
			largest = patch.Size
		}
		steps = append(steps,
			UpdateStep{
				Kind:        StepDownload,
				Label:       "Загрузка патча " + patch.FromVersion + " → " + patch.ToVersion,
				ReleaseID:   patch.ReleaseID,
				PatchID:     patch.ID,
				Bytes:       patch.Size,
				FromVersion: patch.FromVersion,
				ToVersion:   patch.ToVersion,
			},
			UpdateStep{
				Kind:        StepApplyPatch,
				Label:       "Применение патча " + patch.FromVersion + " → " + patch.ToVersion,
				PatchID:     patch.ID,
				FromVersion: patch.FromVersion,
				ToVersion:   patch.ToVersion,
			},
		)
	}
	steps = append(steps, UpdateStep{Kind: StepVerify, Label: "Проверка установки"})

	plan := &UpdatePlan{
		InstalledReleaseID: in.Installed.ReleaseID,
		TargetReleaseID:    in.Target.ID,
		InstalledVersion:   in.Installed.Version,
		TargetVersion:      releaseVersion(in.Target),
		Strategy:           StrategyPatchChain,
		Steps:              steps,
		DownloadBytes:      path.Bytes,
		RequiredDiskBytes:  path.Bytes + largest*2,
		BackupRecommended:  true,
		Confidence:         baseConfidenceOf(in),
		Patches:            path.Steps,
	}
	return plan, nil
}

func (p patchChainStrategy) Apply(ctx context.Context, plan UpdatePlan) error {
	return p.service.applyPatchChain(ctx, plan)
}

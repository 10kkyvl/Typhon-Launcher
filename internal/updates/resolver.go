package updates

import (
	"time"

	"typhon/internal/sources"
	"typhon/internal/version"
)

const (
	updateConfidenceThreshold = 0.6
	newReleaseMinConfidence   = 0.25
)

func versionConfidence(g InstalledGame) float64 {
	if g.VersionConfidence > 0 {
		return clamp(g.VersionConfidence)
	}
	switch g.VersionSource {
	case VersionSourceRelease:
		return 0.95
	case VersionSourceManifest:
		return 0.9
	case VersionSourceManual:
		return 0.8
	case VersionSourceExecutable:
		return 0.7
	default:
		return 0.35
	}
}

func clamp(v float64) float64 {
	switch {
	case v < 0:
		return 0
	case v > 1:
		return 1
	default:
		return v
	}
}

func PatchesFrom(releases []sources.Release) []Patch {
	out := make([]Patch, 0)
	for _, r := range releases {
		if r.Kind != sources.KindPatch || r.Availability != sources.AvailabilityAvailable || r.Ignored {
			continue
		}
		if r.FromVersion == "" || r.ToVersion == "" {
			continue
		}
		patch := Patch{
			ID:             r.ID,
			FromVersion:    r.FromVersion,
			ToVersion:      r.ToVersion,
			ReleaseID:      r.ID,
			SourceID:       r.SourceID,
			DistributionID: r.DistributionID,
			UploadedAt:     r.UploadedAt,
			Title:          r.RawTitle,
			Size:           r.Size,
			Priority:       r.Sequence,
			CreatedAt:      r.CreatedAt,
		}
		if r.CanonicalGameID != nil {
			patch.GameID = *r.CanonicalGameID
		}
		out = append(out, patch)
	}
	return out
}

func usable(installed InstalledGame, r sources.Release) bool {
	if r.Kind == sources.KindPatch || r.Ignored || r.Availability != sources.AvailabilityAvailable {
		return false
	}
	if len(r.URIs) == 0 {
		return false
	}
	if installed.CanonicalGameID != "" {
		return r.CanonicalGameID != nil && *r.CanonicalGameID == installed.CanonicalGameID
	}
	return true
}

// sameDistribution is the trust boundary for update discovery. A feed's
// distributionId is an explicit assertion of continuity and is scoped by the
// source. Older installations may only follow the exact source/release record
// they saved; all metadata similarities are intentionally ignored.
func sameDistribution(installed InstalledGame, r sources.Release) bool {
	if installed.SourceID == "" || installed.ReleaseID == "" || r.SourceID != installed.SourceID {
		return false
	}
	if installed.DistributionID != "" {
		return r.DistributionID != "" && r.DistributionID == installed.DistributionID
	}
	return r.ID == installed.ReleaseID
}

func patchesFor(installed InstalledGame, target sources.Release, patches []Patch) []Patch {
	distributionID := installed.DistributionID
	if distributionID == "" && target.ID == installed.ReleaseID && target.SourceID == installed.SourceID {
		distributionID = target.DistributionID
	}
	if installed.SourceID == "" || distributionID == "" {
		return nil
	}
	out := make([]Patch, 0, len(patches))
	for _, patch := range patches {
		if patch.SourceID == installed.SourceID && patch.DistributionID == distributionID {
			out = append(out, patch)
		}
	}
	return out
}

// ResolveUpdate is deterministic: the same installation, releases and patches
// always produce the same availability.
func ResolveUpdate(installed InstalledGame, releases []sources.Release, patches []Patch) UpdateAvailability {
	out := UpdateAvailability{
		Kind:                       KindNone,
		GameID:                     installed.GameID,
		InstalledReleaseID:         installed.ReleaseID,
		SourceID:                   installed.SourceID,
		DistributionID:             installed.DistributionID,
		InstalledReleaseUploadedAt: installed.ReleaseUploadedAt,
		InstalledVersion:           installed.Version,
	}

	current := version.Parse(installed.Version)
	compatible := make([]sources.Release, 0, len(releases))
	compat := map[string]CompatibilityResult{}
	for _, r := range releases {
		if !sameDistribution(installed, r) || !usable(installed, r) {
			continue
		}
		result := Compatible(installed, r)
		if !result.Compatible {
			continue
		}
		compat[r.ID] = result
		compatible = append(compatible, r)
	}
	if len(compatible) == 0 {
		return out
	}

	ordered := OrderReleases(compatible)
	baseConfidence := versionConfidence(installed)
	installedUpload := revisionBaseline(installed, releases)

	var fallback *sources.Release
	for i := range ordered {
		r := ordered[i]
		if sameInstalledRevision(installed, r) {
			continue
		}
		target := version.Parse(releaseVersion(r))
		newer, ok := version.Newer(target, current)
		if ok && newer {
			return build(installed, r, compat[r.ID], baseConfidence, patchesFor(installed, r, patches), KindUpdate)
		}
		if ok {
			if version.Equal(target, current) && fallback == nil && publishedLater(installedUpload, r.UploadedAt) {
				copied := r
				fallback = &copied
			}
			continue
		}
		if fallback == nil && publishedLater(installedUpload, r.UploadedAt) {
			copied := r
			fallback = &copied
		}
	}

	if fallback == nil {
		return out
	}
	return build(installed, *fallback, compat[fallback.ID], baseConfidence, patchesFor(installed, *fallback, patches), KindNewRelease)
}

// Older installations did not store the source upload date. Their installation
// time is a conservative upper bound: a release uploaded afterwards is newer,
// without pretending that this local timestamp is provider metadata.
func revisionBaseline(installed InstalledGame, releases []sources.Release) *time.Time {
	if installed.ReleaseUploadedAt != nil {
		return installed.ReleaseUploadedAt
	}
	if !installed.InstalledAt.IsZero() {
		return &installed.InstalledAt
	}
	// Some imported legacy records lack installation time as well. Only the
	// exact saved source/release/version can supply a baseline in that case.
	var baseline *time.Time
	matched := false
	for _, release := range releases {
		if release.ID != installed.ReleaseID || !sameDistribution(installed, release) || releaseVersion(release) != installed.Version {
			continue
		}
		if matched {
			return nil
		}
		matched = true
		baseline = release.UploadedAt
	}
	return baseline
}

func sameInstalledRevision(installed InstalledGame, release sources.Release) bool {
	return release.ID == installed.ReleaseID && releaseVersion(release) == installed.Version &&
		samePlanTime(release.UploadedAt, installed.ReleaseUploadedAt)
}

// When versions do not establish an order (incomparable or equal), recency is
// the only argument left for calling a release newer. An equal date is no
// argument at all: two builds uploaded the same day are alternatives, not
// successors.
func publishedLater(installed, candidate *time.Time) bool {
	if installed == nil {
		return false
	}
	if candidate == nil {
		return false
	}
	return candidate.After(*installed)
}

func build(
	installed InstalledGame,
	target sources.Release,
	compat CompatibilityResult,
	baseConfidence float64,
	patches []Patch,
	kind AvailabilityKind,
) UpdateAvailability {
	matchConfidence := target.MatchConfidence
	if matchConfidence <= 0 {
		matchConfidence = 0.5
	}
	confidence := clamp(baseConfidence * matchConfidence * clamp(compat.Confidence))

	out := UpdateAvailability{
		Available:                  true,
		Kind:                       kind,
		GameID:                     installed.GameID,
		InstalledReleaseID:         installed.ReleaseID,
		TargetReleaseID:            target.ID,
		SourceID:                   installed.SourceID,
		DistributionID:             installed.DistributionID,
		InstalledReleaseUploadedAt: installed.ReleaseUploadedAt,
		TargetReleaseUploadedAt:    target.UploadedAt,
		InstalledVersion:           installed.Version,
		TargetVersion:              releaseVersion(target),
		Confidence:                 confidence,
		Strategy:                   StrategyFullRelease,
		EstimatedDownloadBytes:     target.Size,
		RequiresFullInstall:        true,
		TargetSize:                 target.Size,
	}
	if kind == KindNewRelease {
		out.Reason = "new_distribution_revision"
	}
	if kind == KindUpdate && confidence < updateConfidenceThreshold {
		out.Kind = KindNewRelease
		out.Reason = "низкая уверенность в сопоставлении версий"
	}
	if out.Kind == KindNewRelease && confidence < newReleaseMinConfidence {
		return UpdateAvailability{
			Kind:                       KindNone,
			GameID:                     installed.GameID,
			InstalledReleaseID:         installed.ReleaseID,
			SourceID:                   installed.SourceID,
			DistributionID:             installed.DistributionID,
			InstalledReleaseUploadedAt: installed.ReleaseUploadedAt,
			InstalledVersion:           installed.Version,
		}
	}
	if len(compat.Reasons) > 0 && out.Reason == "" {
		out.Reason = compat.Reasons[0]
	}

	if path, ok := FindPatchPath(patches, installed.Version, out.TargetVersion); ok {
		if out.EstimatedDownloadBytes <= 0 || path.Bytes < out.EstimatedDownloadBytes {
			out.Strategy = StrategyPatchChain
			out.EstimatedDownloadBytes = path.Bytes
			out.RequiresFullInstall = false
			out.PatchCount = len(path.Steps)
		}
	}
	return out
}

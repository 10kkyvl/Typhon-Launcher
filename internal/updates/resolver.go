package updates

import (
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
			ID:          r.ID,
			FromVersion: r.FromVersion,
			ToVersion:   r.ToVersion,
			ReleaseID:   r.ID,
			SourceID:    r.SourceID,
			Title:       r.RawTitle,
			Size:        r.Size,
			Priority:    r.Sequence,
			CreatedAt:   r.CreatedAt,
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

// ResolveUpdate is deterministic: the same installation, releases and patches
// always produce the same availability.
func ResolveUpdate(installed InstalledGame, releases []sources.Release, patches []Patch) UpdateAvailability {
	out := UpdateAvailability{
		Kind:               KindNone,
		GameID:             installed.GameID,
		InstalledReleaseID: installed.ReleaseID,
		InstalledVersion:   installed.Version,
	}

	current := version.Parse(installed.Version)
	compatible := make([]sources.Release, 0, len(releases))
	compat := map[string]CompatibilityResult{}
	for _, r := range releases {
		if !usable(installed, r) {
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

	var fallback *sources.Release
	for i := range ordered {
		r := ordered[i]
		if r.ID == installed.ReleaseID {
			break
		}
		target := version.Parse(releaseVersion(r))
		newer, ok := version.Newer(target, current)
		if ok && newer {
			return build(installed, r, current, target, compat[r.ID], baseConfidence, patches, KindUpdate)
		}
		if ok {
			continue
		}
		if fallback == nil {
			copied := r
			fallback = &copied
		}
	}

	if fallback == nil {
		return out
	}
	target := version.Parse(releaseVersion(*fallback))
	return build(installed, *fallback, current, target, compat[fallback.ID], baseConfidence, patches, KindNewRelease)
}

func build(
	installed InstalledGame,
	target sources.Release,
	currentVersion, targetVersion version.Version,
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
		Available:              true,
		Kind:                   kind,
		GameID:                 installed.GameID,
		InstalledReleaseID:     installed.ReleaseID,
		TargetReleaseID:        target.ID,
		InstalledVersion:       installed.Version,
		TargetVersion:          releaseVersion(target),
		Confidence:             confidence,
		Strategy:               StrategyFullRelease,
		EstimatedDownloadBytes: target.Size,
		RequiresFullInstall:    true,
		TargetSize:             target.Size,
	}
	if kind == KindUpdate && confidence < updateConfidenceThreshold {
		out.Kind = KindNewRelease
		out.Reason = "низкая уверенность в сопоставлении версий"
	}
	if out.Kind == KindNewRelease && out.Reason == "" {
		if !currentVersion.Comparable || !targetVersion.Comparable {
			out.Reason = "версии нельзя сравнить"
		} else {
			out.Reason = "версии из разных схем нумерации"
		}
	}
	if out.Kind == KindNewRelease && confidence < newReleaseMinConfidence {
		return UpdateAvailability{
			Kind:               KindNone,
			GameID:             installed.GameID,
			InstalledReleaseID: installed.ReleaseID,
			InstalledVersion:   installed.Version,
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

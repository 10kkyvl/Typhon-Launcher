package updates

import (
	"strings"

	"typhon/internal/sources"
	"typhon/internal/titles"
)

const (
	confidenceExactEdition   = 1.0
	confidenceUnknownEdition = 0.85
	confidenceLanguageMiss   = 0.9
	confidenceSmallerRelease = 0.5

	// A download this much smaller than the installed game is a different kind of
	// build, not the same game one version on. Repacks compress hard, so the
	// ratio stays well clear of what an honest repack achieves.
	smallReleaseRatio = 4
)

func Compatible(installed InstalledGame, r sources.Release) CompatibilityResult {
	result := CompatibilityResult{Compatible: true, Confidence: confidenceExactEdition}

	current := titles.Normalize(installed.Edition)
	target := titles.Normalize(r.Edition)
	switch {
	case current != "" && target != "" && current != target:
		return CompatibilityResult{Confidence: 0, Reasons: []string{"edition: " + r.Edition}}
	case current == "" && target != "":
		result.Confidence = confidenceUnknownEdition
		result.Reasons = append(result.Reasons, "edition: "+r.Edition)
	case current != "" && target == "":
		result.Confidence = confidenceUnknownEdition
		result.Reasons = append(result.Reasons, "edition unknown")
	}

	if len(installed.Languages) > 0 && len(r.Languages) > 0 && !sharesLanguage(installed.Languages, r.Languages) {
		result.Confidence *= confidenceLanguageMiss
		result.Reasons = append(result.Reasons, "language: "+strings.Join(r.Languages, ", "))
	}

	if muchSmaller(installed.SizeBytes, r.Size) {
		result.Confidence *= confidenceSmallerRelease
		result.Reasons = append(result.Reasons, "размер раздачи сильно меньше установленной игры")
	}
	return result
}

func muchSmaller(installedBytes, releaseBytes int64) bool {
	if installedBytes <= 0 || releaseBytes <= 0 {
		return false
	}
	return releaseBytes*smallReleaseRatio < installedBytes
}

func sharesLanguage(a, b []string) bool {
	set := make(map[string]bool, len(a))
	for _, l := range a {
		set[strings.ToUpper(strings.TrimSpace(l))] = true
	}
	for _, l := range b {
		code := strings.ToUpper(strings.TrimSpace(l))
		if set[code] || strings.HasPrefix(code, "MULTI") {
			return true
		}
	}
	for l := range set {
		if strings.HasPrefix(l, "MULTI") {
			return true
		}
	}
	return false
}

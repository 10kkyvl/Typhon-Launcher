package updates

import (
	"strings"
	"testing"
	"time"

	"typhon/internal/sources"
)

func TestCompatibleFlagsMuchSmallerRelease(t *testing.T) {
	installed := installedAt("r1", "1.0")
	installed.SizeBytes = 10518974050
	got := Compatible(installed, release("r2", "1.1", 1820000000))

	if !got.Compatible {
		t.Fatalf("a smaller release stays a candidate, just a weaker one: %+v", got)
	}
	if got.Confidence >= confidenceExactEdition {
		t.Fatalf("confidence = %v, want it lowered", got.Confidence)
	}
	if len(got.Reasons) == 0 || !strings.Contains(strings.Join(got.Reasons, " "), "размер") {
		t.Fatalf("reasons = %v, want the size mentioned", got.Reasons)
	}
}

func TestCompatibleKeepsComparableSizeAtFullConfidence(t *testing.T) {
	installed := installedAt("r1", "1.0")
	installed.SizeBytes = 10518974050
	got := Compatible(installed, release("r2", "1.1", 9790000000))

	if got.Confidence != confidenceExactEdition {
		t.Fatalf("confidence = %v, want %v", got.Confidence, confidenceExactEdition)
	}
}

// A repack an order of magnitude smaller than the install is an alternative
// build, not an update, even when it was published later.
func TestResolveUpdateSuppressesMuchSmallerRepack(t *testing.T) {
	older := time.Date(2026, 3, 25, 11, 9, 14, 0, time.UTC)
	newer := older.Add(48 * time.Hour)
	installedRelease := release("r1-portable", "", 9790000000)
	installedRelease.UploadedAt = &older
	repack := release("r2-repack", "", 1820000000)
	repack.UploadedAt = &newer

	installed := installedAt("r1-portable", "")
	installed.VersionSource = VersionSourceUnknown
	installed.SizeBytes = 10518974050
	got := ResolveUpdate(installed, []sources.Release{installedRelease, repack}, nil)

	if got.Available || got.Kind != KindNone {
		t.Fatalf("a much smaller repack must not be offered: %+v", got)
	}
}

func TestResolveUpdateDowngradesMuchSmallerReleaseWithNewerVersion(t *testing.T) {
	installed := installedAt("r1", "1.0")
	installed.SizeBytes = 10518974050
	got := ResolveUpdate(installed, []sources.Release{
		release("r1", "1.0", 9790000000),
		release("r2", "1.1", 1820000000),
	}, nil)

	if !got.Available {
		t.Fatalf("a newer version stays visible: %+v", got)
	}
	if got.Kind != KindNewRelease {
		t.Fatalf("kind = %q, want %q", got.Kind, KindNewRelease)
	}
}

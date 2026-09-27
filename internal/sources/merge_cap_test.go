package sources

import (
	"fmt"
	"testing"
	"time"
)

// TestMergeEvictsOldestRemovedPastCap guards the second audit finding:
// merge() marks vanished entries AvailabilityRemoved but never dropped them,
// so a source with churn (entries leaving and returning over months) could
// grow s.releases[id] without any upper bound — feed.MaxEntries only caps a
// single parse pass, not the accumulated list. Once the number of removed
// releases exceeds maxRemovedReleases, the oldest-by-LastSeenAt ones must be
// evicted; releases still available in the feed are never touched.
func TestMergeEvictsOldestRemovedPastCap(t *testing.T) {
	now := time.Now()

	existing := make([]*Release, 0, maxRemovedReleases+2)
	for i := 0; i < maxRemovedReleases+1; i++ {
		existing = append(existing, &Release{
			ID:              fmt.Sprintf("removed-%d", i),
			NormalizedTitle: fmt.Sprintf("game %d", i),
			RawTitle:        fmt.Sprintf("Game %d", i),
			Availability:    AvailabilityRemoved,
			// Distinct, increasing LastSeenAt: index 0 disappeared longest
			// ago and must be the first one evicted.
			LastSeenAt: now.Add(time.Duration(i) * time.Minute),
		})
	}
	// One release still available in the feed must survive no matter how
	// many removed releases pile up around it.
	existing = append(existing, &Release{
		ID:              "still-available",
		NormalizedTitle: "still available",
		RawTitle:        "Still Available",
		Availability:    AvailabilityAvailable,
		LastSeenAt:      now,
	})

	// A matching incoming entry keeps "still-available" present this round;
	// everything else stays absent from the feed and therefore removed.
	incoming := []*Release{{
		NormalizedTitle: "still available",
		RawTitle:        "Still Available",
		Availability:    AvailabilityAvailable,
	}}

	merged, _ := merge(existing, incoming, now.Add(24*time.Hour), false)

	removed := 0
	byID := map[string]*Release{}
	for _, r := range merged {
		byID[r.ID] = r
		if r.Availability == AvailabilityRemoved {
			removed++
		}
	}
	if removed != maxRemovedReleases {
		t.Fatalf("removed releases retained = %d, want the cap %d", removed, maxRemovedReleases)
	}
	if _, ok := byID["removed-0"]; ok {
		t.Fatal("the oldest removed release (removed-0) should have been evicted first")
	}
	newestRemoved := fmt.Sprintf("removed-%d", maxRemovedReleases)
	if _, ok := byID[newestRemoved]; !ok {
		t.Fatalf("the most recently seen removed release (%s) should not be evicted", newestRemoved)
	}
	if _, ok := byID["still-available"]; !ok {
		t.Fatal("an available release must never be evicted, regardless of how many removed releases exist")
	}
}

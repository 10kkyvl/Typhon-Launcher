package sources

import (
	"testing"
	"time"
)

// TestPruneExpiredPreviewDropsStaleCache guards the third audit finding:
// cachedFeed's TTL was only checked lazily inside takeCachedFeed, so a
// preview nobody ever asked for again (wizard opened, "Проверить" clicked,
// wizard closed) kept its ~34 MB parsed feed.Result alive until the next
// TestSource/TestSourceFile call or a full restart, not for previewCacheTTL.
// pruneExpiredPreview must free the reference once it is older than the TTL
// on its own, independent of takeCachedFeed ever running again.
func TestPruneExpiredPreviewDropsStaleCache(t *testing.T) {
	s := &Service{}
	at := time.Now().Add(-previewCacheTTL - time.Minute)
	s.cached = &cachedFeed{kind: TypeURL, location: "https://example.test/feed.json", at: at}

	s.pruneExpiredPreview(time.Now())

	if s.cached != nil {
		t.Fatal("preview cache past its TTL should have been dropped")
	}
}

// TestPruneExpiredPreviewKeepsFreshCache makes sure pruning does not evict a
// preview that is still within its TTL: the slot exists precisely so
// AddSource right after TestSource does not re-download the feed.
func TestPruneExpiredPreviewKeepsFreshCache(t *testing.T) {
	s := &Service{}
	at := time.Now().Add(-previewCacheTTL / 2)
	s.cached = &cachedFeed{kind: TypeURL, location: "https://example.test/feed.json", at: at}

	s.pruneExpiredPreview(time.Now())

	if s.cached == nil {
		t.Fatal("preview cache still within its TTL should not have been dropped")
	}
}

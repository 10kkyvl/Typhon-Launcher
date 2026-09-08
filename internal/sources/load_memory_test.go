package sources

import (
	"fmt"
	"runtime"
	"runtime/debug"
	"testing"
	"time"

	"typhon/internal/storage"
)

// TestLoadDoesNotDoubleMaterializeReleases guards against service.load()
// building both the []Release slice storage.Load unmarshals into and a
// second []*Release slice of per-element copies. The audit measured this as
// an extra ~46 MB per source at 100k releases (HeapAlloc rose from ~198 MB
// right after ReadFile+Unmarshal to ~244.58 MB after the copy loop): the
// value slice storage.Load just built stays live for as long as the copy
// loop that turns it into pointers runs.
//
// The test compares two heap-growth measurements on the same fixture: a
// "doubled" reference that deliberately reproduces the removed pattern
// (storage.Load into a value slice, then copy into a pointer slice) against
// the real s.load(). A single materialization must grow the heap
// meaningfully less than the deliberately doubled one; testing.AllocsPerRun
// cannot see this because swapping the []Release unmarshal target for
// []*Release keeps the allocation *count* roughly the same (N pointer
// targets instead of one bulk array) — only the byte total halves.
func TestLoadDoesNotDoubleMaterializeReleases(t *testing.T) {
	dir := t.TempDir()
	st := newStore(dir)
	const n = 20000
	if err := st.saveSources([]Source{{ID: "src-1", Name: "Feed", URL: "https://example.test/feed.json"}}); err != nil {
		t.Fatalf("save sources: %v", err)
	}

	now := time.Now()
	list := make([]*Release, 0, n)
	for i := 0; i < n; i++ {
		list = append(list, &Release{
			ID:              fmt.Sprintf("release-%d", i),
			SourceID:        "src-1",
			RawTitle:        fmt.Sprintf("Some.Game.Title.%d.v1.0.MULTi10.Repack", i),
			Title:           fmt.Sprintf("Some Game Title %d", i),
			NormalizedTitle: fmt.Sprintf("some game title %d", i),
			Version:         "1.0",
			URIs:            []string{fmt.Sprintf("magnet:?xt=urn:btih:%040d", i)},
			Tags:            []string{"repack", "multi10"},
			Availability:    AvailabilityAvailable,
			FirstSeenAt:     now,
			LastSeenAt:      now,
			CreatedAt:       now,
		})
	}
	if err := st.saveReleases("src-1", list); err != nil {
		t.Fatalf("save releases: %v", err)
	}

	restore := debug.SetGCPercent(-1)
	defer debug.SetGCPercent(restore)

	measure := func(f func()) uint64 {
		runtime.GC()
		var before, after runtime.MemStats
		runtime.ReadMemStats(&before)
		f()
		runtime.ReadMemStats(&after)
		return after.HeapAlloc - before.HeapAlloc
	}

	// Deliberately reproduces the pattern service.load() used to have:
	// unmarshal into a value slice, then copy every element into a second,
	// pointer slice.
	var doubled []*Release
	doubledGrowth := measure(func() {
		var stored []Release
		if err := storage.Load(st.releasesPath("src-1"), releasesVersion, nil, &stored); err != nil {
			t.Fatalf("load stored: %v", err)
		}
		out := make([]*Release, 0, len(stored))
		for i := range stored {
			r := stored[i]
			out = append(out, &r)
		}
		doubled = out
	})
	if len(doubled) != n {
		t.Fatalf("doubled reference loaded = %d releases, want %d", len(doubled), n)
	}

	var single []*Release
	singleGrowth := measure(func() {
		s := &Service{store: newStore(dir), releases: map[string][]*Release{}}
		if err := s.load(); err != nil {
			t.Fatalf("load: %v", err)
		}
		single = s.releases["src-1"]
	})
	if len(single) != n {
		t.Fatalf("s.load() loaded = %d releases, want %d", len(single), n)
	}

	t.Logf("doubled materialization grew the heap by %d bytes; s.load() grew it by %d bytes (%.1f%%)",
		doubledGrowth, singleGrowth, 100*float64(singleGrowth)/float64(doubledGrowth))

	// A single materialization must not even come close to the doubled
	// reference; 80% leaves headroom for decode overhead common to both
	// paths while still catching a load() that materializes the list twice.
	if float64(singleGrowth) > float64(doubledGrowth)*0.8 {
		t.Fatalf("s.load() heap growth (%d bytes) is too close to the deliberately doubled reference (%d bytes): "+
			"it must materialize the release list once, not twice", singleGrowth, doubledGrowth)
	}
}

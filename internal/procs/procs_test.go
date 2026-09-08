package procs

import (
	"context"
	"runtime"
	"testing"
	"time"

	"typhon/internal/devmock"
)

// Перечисление процессов поддержано там, где есть чему их перечислять:
// Windows нативно, macOS через бутыли CrossOver, devmock — фейковым реестром.
func TestSupportedMatchesGOOS(t *testing.T) {
	want := runtime.GOOS == "windows" || runtime.GOOS == "darwin" || devmock.Enabled
	if got := Supported(); got != want {
		t.Fatalf("Supported() = %v, want %v for GOOS=%s", got, want, runtime.GOOS)
	}
}

func TestListOnUnsupportedPlatform(t *testing.T) {
	if Supported() {
		t.Skip("this case is exercised on platforms without process enumeration only")
	}
	got, _, err := List(context.Background())
	if err == nil {
		t.Fatalf("List() on %s returned nil error, want an error", runtime.GOOS)
	}
	if got != nil {
		t.Fatalf("List() on %s returned %v processes alongside an error, want nil", runtime.GOOS, got)
	}
}

func TestImageCacheHitReturnsStoredPathForMatchingCreationTime(t *testing.T) {
	c := newImageCache()
	created := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	c.store(100, created, `C:\Games\Foo\foo.exe`)

	path, ok := c.lookup(100, created)
	if !ok {
		t.Fatal("lookup() ok = false, want true for a pid stored under the same creation time")
	}
	if path != `C:\Games\Foo\foo.exe` {
		t.Fatalf("lookup() path = %q, want the stored path", path)
	}
}

func TestImageCacheMissesOnRecycledPid(t *testing.T) {
	c := newImageCache()
	original := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	c.store(100, original, `C:\Games\Foo\foo.exe`)

	// A different creation time for the same pid means the OS recycled it
	// for an unrelated process: the old path must not be handed back.
	recycled := original.Add(time.Hour)
	if _, ok := c.lookup(100, recycled); ok {
		t.Fatal("lookup() ok = true for a pid whose creation time changed, want false (recycled pid)")
	}
}

func TestImageCachePrunesPidsMissingFromLiveSet(t *testing.T) {
	c := newImageCache()
	created := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	c.store(100, created, `C:\Games\Foo\foo.exe`)
	c.store(200, created, `C:\Games\Bar\bar.exe`)

	c.prune(map[uint32]struct{}{100: {}})

	if _, ok := c.lookup(100, created); !ok {
		t.Fatal("prune() evicted a pid that is still in the live set")
	}
	if _, ok := c.lookup(200, created); ok {
		t.Fatal("prune() kept a pid that is no longer in the live set")
	}
	if got := len(c.entries); got != 1 {
		t.Fatalf("cache holds %d entries after prune, want 1 (unbounded growth from dead pids)", got)
	}
}

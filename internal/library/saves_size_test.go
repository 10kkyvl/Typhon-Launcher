package library

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"typhon/internal/platform"
)

func writeSized(t *testing.T, root, rel string, size int) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, make([]byte, size), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestDetectSavesRejectsCacheFolders(t *testing.T) {
	tests := []struct {
		name      string
		files     map[string]int
		maxBytes  int64
		maxFiles  int
		wantFound bool
	}{
		{
			name:      "small saves accepted",
			files:     map[string]int{"slot1.sav": 100, "slot2.sav": 100},
			maxBytes:  1000,
			maxFiles:  10,
			wantFound: true,
		},
		{
			name:     "over the size limit rejected",
			files:    map[string]int{"a.bin": 600, "b.bin": 600},
			maxBytes: 1000,
			maxFiles: 10,
		},
		{
			name:      "over the file limit judged by what was counted",
			files:     map[string]int{"1": 1, "2": 1, "3": 1, "4": 1},
			maxBytes:  1000,
			maxFiles:  3,
			wantFound: true,
		},
		{
			name:     "cache seen before the file limit still rejects",
			files:    map[string]int{"web_cache/a": 50, "web_cache/b": 50, "web_cache/c": 50, "x": 1},
			maxBytes: 1000,
			maxFiles: 3,
		},
		{
			name:     "mostly cache directories rejected",
			files:    map[string]int{"web_cache/x": 600, "profile/p": 100},
			maxBytes: 1000,
			maxFiles: 10,
		},
		{
			name:     "nested logs count as cache",
			files:    map[string]int{"a/Logs/x": 600, "save.dat": 100},
			maxBytes: 1000,
			maxFiles: 10,
		},
		{
			name:      "minority cache accepted",
			files:     map[string]int{"cache/x": 100, "save.dat": 600},
			maxBytes:  1000,
			maxFiles:  10,
			wantFound: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			oldBytes, oldFiles := maxHeuristicSavesBytes, maxHeuristicSavesEntries
			maxHeuristicSavesBytes, maxHeuristicSavesEntries = tt.maxBytes, tt.maxFiles
			t.Cleanup(func() { maxHeuristicSavesBytes, maxHeuristicSavesEntries = oldBytes, oldFiles })

			root := t.TempDir()
			for rel, size := range tt.files {
				writeSized(t, filepath.Join(root, "roots", "Portal"), rel, size)
			}
			roots := func() ([]platform.SaveRoot, error) {
				return []platform.SaveRoot{{Path: filepath.Join(root, "roots"), Depth: 1}}, nil
			}
			result, err := detectSaves(t.Context(), "Portal", "", roots)
			if err != nil {
				t.Fatalf("detect: %v", err)
			}
			found := result.Path != "" || len(result.Candidates) > 0
			if found != tt.wantFound {
				t.Fatalf("found = %v, want %v (%+v)", found, tt.wantFound, result)
			}
			if !tt.wantFound && result.Unreadable != 0 {
				t.Fatalf("rejection counted as unreadable: %+v", result)
			}
		})
	}
}

func TestVetHeuristicSavesStopsAtEntryLimit(t *testing.T) {
	oldFiles := maxHeuristicSavesEntries
	maxHeuristicSavesEntries = 2
	t.Cleanup(func() { maxHeuristicSavesEntries = oldFiles })

	root := t.TempDir()
	for _, n := range []string{"a", "b", "c", "d", "e"} {
		writeSized(t, root, n, 1)
	}
	seen := 0
	withWalk(t, func(r string, fn fs.WalkDirFunc) error {
		return filepath.WalkDir(r, func(p string, d fs.DirEntry, err error) error {
			if err == nil && !d.IsDir() {
				seen++
			}
			return fn(p, d, err)
		})
	})
	verdict, _, err := vetHeuristicSaves(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	if verdict.reason != "" || seen != 2 {
		t.Fatalf("reason = %q, files visited = %d, want accepted after 2", verdict.reason, seen)
	}
}

func TestVetHeuristicSavesCancelledContext(t *testing.T) {
	root := t.TempDir()
	writeSized(t, root, "a", 1)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := vetHeuristicSaves(ctx, root); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

func TestLocateSavesHintAndStoredDirSkipSizeLimit(t *testing.T) {
	oldBytes := maxHeuristicSavesBytes
	maxHeuristicSavesBytes = 10
	t.Cleanup(func() { maxHeuristicSavesBytes = oldBytes })

	root := t.TempDir()
	mkdirs(t, root, "install")
	writeSized(t, root, "picked/big.bin", 500)
	s := savesService(t, nil)
	game := addSavesGame(t, s, "Portal", filepath.Join(root, "install"))
	if _, err := s.SetSavesDir(game.ID, filepath.Join(root, "picked")); err != nil {
		t.Fatal(err)
	}
	result, err := s.LocateSaves(t.Context(), game.ID)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(root, "picked"); result.Path != want {
		t.Fatalf("stored path = %q, want %q", result.Path, want)
	}

	f := newHintFixture(t, "Prey")
	writeSized(t, f.root, "Roaming/Arkane/Prey/big.bin", 500)
	f.hints = []string{"{appdata}/Arkane/Prey"}
	hinted, err := f.s.LocateSaves(t.Context(), f.game.ID)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(f.root, "Roaming", "Arkane", "Prey"); hinted.Path != want {
		t.Fatalf("hinted path = %q, want %q", hinted.Path, want)
	}
}

func withWalk(t *testing.T, walk func(string, fs.WalkDirFunc) error) {
	t.Helper()
	old := walkDir
	walkDir = walk
	t.Cleanup(func() { walkDir = old })
}

func portalRoots(root string) func() ([]platform.SaveRoot, error) {
	return func() ([]platform.SaveRoot, error) {
		return []platform.SaveRoot{{Path: filepath.Join(root, "roots"), Depth: 1}}, nil
	}
}

func TestDetectSavesAcceptsLargeRealSaves(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, "roots", "Portal", "world", "region.dat")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(300 << 20); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	writeSized(t, root, "roots/Portal/logs/a.log", 10)

	result, err := detectSaves(t.Context(), "Portal", "", portalRoots(root))
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(root, "roots", "Portal"); result.Path != want || len(result.Rejected) != 0 {
		t.Fatalf("result = %+v, want path %q and nothing rejected", result, want)
	}
}

func TestDetectSavesReportsRejectedCache(t *testing.T) {
	root := t.TempDir()
	writeSized(t, root, "roots/Portal/res_cache/a.bin", 700)
	writeSized(t, root, "roots/Portal/profile.dat", 100)
	result, err := detectSaves(t.Context(), "Portal", "", portalRoots(root))
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "roots", "Portal")
	if result.Path != "" || len(result.Candidates) != 0 {
		t.Fatalf("cache accepted: %+v", result)
	}
	if len(result.Rejected) != 1 || result.Rejected[0].Path != dir || result.Rejected[0].Reason == "" {
		t.Fatalf("rejected = %+v, want %q with a reason", result.Rejected, dir)
	}
}

func TestDetectSavesKeepsAcceptedNextToRejected(t *testing.T) {
	root := t.TempDir()
	writeSized(t, root, "roots/Portal/res_cache/a.bin", 700)
	writeSized(t, root, "roots/Portal Saves/slot.sav", 100)

	result, err := detectSaves(t.Context(), "Portal", "", portalRoots(root))
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(root, "roots", "Portal Saves"); result.Path != want {
		t.Fatalf("path = %q, want %q (%+v)", result.Path, want, result)
	}
	if len(result.Rejected) != 1 {
		t.Fatalf("rejected = %+v, want the cache recorded", result.Rejected)
	}
}

func TestDetectSavesSurvivesWalkFailure(t *testing.T) {
	root := t.TempDir()
	writeSized(t, root, "roots/Portal/slot.sav", 10)
	withWalk(t, func(string, fs.WalkDirFunc) error { return errors.New("path too long") })

	result, err := detectSaves(t.Context(), "Portal", "", portalRoots(root))
	if err != nil {
		t.Fatalf("a failed measurement must not fail the search: %v", err)
	}
	if want := filepath.Join(root, "roots", "Portal"); result.Path != want || result.Unreadable != 1 {
		t.Fatalf("result = %+v, want path %q accepted with 1 unreadable", result, want)
	}
}

func TestDetectSavesWalkFailureWithCancelledContext(t *testing.T) {
	root := t.TempDir()
	writeSized(t, root, "roots/Portal/slot.sav", 10)
	ctx, cancel := context.WithCancel(context.Background())
	withWalk(t, func(string, fs.WalkDirFunc) error {
		cancel()
		return errors.New("interrupted")
	})
	if _, err := detectSaves(ctx, "Portal", "", portalRoots(root)); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

func TestVetHeuristicSavesCachesVerdict(t *testing.T) {
	root := t.TempDir()
	writeSized(t, root, "res_cache/a.bin", 700)
	writeSized(t, root, "p.dat", 100)
	walks := 0
	withWalk(t, func(r string, fn fs.WalkDirFunc) error {
		walks++
		return filepath.WalkDir(r, fn)
	})

	first, cached, err := vetHeuristicSaves(t.Context(), root)
	if err != nil || cached || first.reason == "" {
		t.Fatalf("first = %+v cached=%v err=%v", first, cached, err)
	}
	second, cached, err := vetHeuristicSaves(t.Context(), root)
	if err != nil || !cached || second != first || walks != 1 {
		t.Fatalf("second = %+v cached=%v walks=%d err=%v", second, cached, walks, err)
	}

	writeSized(t, root, "new.dat", 1)
	if _, cached, err = vetHeuristicSaves(t.Context(), root); err != nil || cached || walks != 2 {
		t.Fatalf("changed folder reused cache: cached=%v walks=%d err=%v", cached, walks, err)
	}
}

func TestVerdictCacheIsBounded(t *testing.T) {
	for i := range verdictCacheLimit + 5 {
		storeVerdict(filepath.Join("bounded", string(rune('a'+i%26)), string(rune('0'+i/26))), verdictKey{entries: i}, heuristicVerdict{})
	}
	verdictMu.Lock()
	n := len(verdictCache)
	verdictMu.Unlock()
	if n > verdictCacheLimit {
		t.Fatalf("cache grew to %d entries", n)
	}
}

func TestSavesLimitMatchesCloudLimit(t *testing.T) {
	if maxHeuristicSavesBytes < 1<<30 {
		t.Fatalf("size limit %d cuts real saves below 1 GiB", maxHeuristicSavesBytes)
	}
}

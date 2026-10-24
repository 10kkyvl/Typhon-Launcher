package library

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
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
			name:     "over the file limit rejected",
			files:    map[string]int{"1": 1, "2": 1, "3": 1, "4": 1},
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

func TestVetHeuristicSavesStopsAtLimit(t *testing.T) {
	oldFiles := maxHeuristicSavesEntries
	maxHeuristicSavesEntries = 2
	t.Cleanup(func() { maxHeuristicSavesEntries = oldFiles })

	root := t.TempDir()
	for _, n := range []string{"a", "b", "c", "d", "e"} {
		writeSized(t, root, n, 1)
	}
	verdict, err := vetHeuristicSaves(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(verdict.reason, "files") {
		t.Fatalf("reason = %q", verdict.reason)
	}
}

func TestVetHeuristicSavesCancelledContext(t *testing.T) {
	root := t.TempDir()
	writeSized(t, root, "a", 1)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := vetHeuristicSaves(ctx, root); !errors.Is(err, context.Canceled) {
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

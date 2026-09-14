package profile

import (
	"testing"
	"time"
	"typhon/internal/library"
	"typhon/internal/playlog"
)

type previewLibrary struct{}

func (previewLibrary) GetGames() []library.Game {
	return []library.Game{{ID: "a", Title: "A", Favorite: true, Status: "completed", PlaytimeSeconds: 7200}}
}
func (previewLibrary) GetRunningGames() []string { return nil }

type previewLog struct{}

func (previewLog) Since(time.Time) []playlog.Session { return nil }

func TestPreviewIncludesDisabledShowcasesWithoutSaving(t *testing.T) {
	s := NewService(previewLibrary{}, previewLog{}, func() []string { return []string{"favorites"} })
	preview := s.Preview()
	if len(preview.Showcase) != 3 {
		t.Fatalf("preview missing disabled showcases: %+v", preview.Showcase)
	}
	for _, block := range preview.Showcase {
		if len(block.Games) != 1 {
			t.Fatalf("preview missing games: %+v", block)
		}
	}
	saved := s.Snapshot()
	if len(saved.Showcase) != 1 || saved.Showcase[0].Kind != "favorites" {
		t.Fatalf("preview mutated saved order: %+v", saved.Showcase)
	}
}

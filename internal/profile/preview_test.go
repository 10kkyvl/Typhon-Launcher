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
	s, err := NewService(previewLibrary{}, previewLog{}, stubCatalog{}, func() []string { return []string{"favorites"} }, noLayout)
	if err != nil {
		t.Fatal(err)
	}
	preview, err := s.Preview()
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Showcase) != 3 {
		t.Fatalf("preview missing disabled showcases: %+v", preview.Showcase)
	}
	for _, block := range preview.Showcase {
		if len(block.Games) != 1 {
			t.Fatalf("preview missing games: %+v", block)
		}
	}
	saved, err := s.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if len(saved.Showcase) != 1 || saved.Showcase[0].Kind != "favorites" {
		t.Fatalf("preview mutated saved order: %+v", saved.Showcase)
	}
}

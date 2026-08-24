package catalog

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"typhon/internal/titles"
)

func metadataService(t *testing.T) (*Service, string) {
	t.Helper()
	dir := t.TempDir()
	svc, err := NewServiceAt(dir)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	return svc, dir
}

func samplePatch() MetadataPatch {
	released := time.Date(2017, time.May, 5, 0, 0, 0, 0, time.UTC)
	return MetadataPatch{
		IGDBID:       "2657",
		Title:        "Prey",
		Summary:      "Станция «Талос-1».",
		ReleaseDate:  &released,
		Developer:    "Arkane Studios",
		Publisher:    "Bethesda Softworks",
		Genres:       []string{"Shooter", " ", "Adventure"},
		Themes:       []string{"Science fiction"},
		Platforms:    []string{"PC (Microsoft Windows)"},
		CoverAssetID: "cover1",
		HeroAssetID:  "hero1",
		UpdatedAt:    time.Now(),
	}
}

func TestApplyMetadataStoresFields(t *testing.T) {
	svc, dir := metadataService(t)
	game, err := svc.AddGame(Game{Title: "Prey"})
	if err != nil {
		t.Fatalf("add game: %v", err)
	}

	updated, err := svc.ApplyMetadata(game.ID, samplePatch())
	if err != nil {
		t.Fatalf("apply metadata: %v", err)
	}

	if updated.ExternalIDs.IGDB != "2657" {
		t.Fatalf("igdb id = %q", updated.ExternalIDs.IGDB)
	}
	if updated.Summary == "" || updated.Developer != "Arkane Studios" || updated.Publisher != "Bethesda Softworks" {
		t.Fatalf("text metadata = %+v", updated)
	}
	if len(updated.Genres) != 2 {
		t.Fatalf("genres = %v, want blank entries dropped", updated.Genres)
	}
	if updated.ReleaseDate == nil || updated.ReleaseDate.Year() != 2017 {
		t.Fatalf("release date = %v", updated.ReleaseDate)
	}
	if updated.ReleaseYear == nil || *updated.ReleaseYear != 2017 {
		t.Fatalf("release year = %v", updated.ReleaseYear)
	}
	if updated.CoverAssetID != "cover1" || updated.HeroAssetID != "hero1" {
		t.Fatalf("asset ids = %q/%q", updated.CoverAssetID, updated.HeroAssetID)
	}
	if updated.MetadataUpdatedAt == nil {
		t.Fatal("metadata timestamp missing")
	}

	reopened, err := NewServiceAt(dir)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	stored, err := reopened.GetGame(game.ID)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if stored.Summary != updated.Summary || stored.ExternalIDs.IGDB != "2657" {
		t.Fatalf("metadata not persisted: %+v", stored)
	}
}

func TestApplyMetadataRenamesProvisionalGameAndKeepsAlias(t *testing.T) {
	svc, _ := metadataService(t)
	game, err := svc.AddGame(Game{Title: "Prey 2017 Repack", Provisional: true})
	if err != nil {
		t.Fatalf("add game: %v", err)
	}
	oldNormalized := titles.Normalize(game.Title)

	updated, err := svc.ApplyMetadata(game.ID, samplePatch())
	if err != nil {
		t.Fatalf("apply metadata: %v", err)
	}
	if updated.Title != "Prey" {
		t.Fatalf("title = %q, want the provider title", updated.Title)
	}
	if updated.Provisional {
		t.Fatal("game is still provisional after a confirmed match")
	}
	if updated.SortTitle != "prey" {
		t.Fatalf("sort title = %q", updated.SortTitle)
	}

	found, ok := svc.LookupByTitle(oldNormalized)
	if !ok || found.ID != game.ID {
		t.Fatalf("release matching broken: the previous title no longer resolves (%v)", ok)
	}
	match := svc.Resolve(Query{Title: "Prey 2017 Repack"})
	if match.Status != StatusMatched || match.GameID != game.ID {
		t.Fatalf("resolve = %+v", match)
	}
}

func TestApplyMetadataKeepsConfirmedTitle(t *testing.T) {
	svc, _ := metadataService(t)
	game, err := svc.AddGame(Game{Title: "Прей"})
	if err != nil {
		t.Fatalf("add game: %v", err)
	}

	updated, err := svc.ApplyMetadata(game.ID, samplePatch())
	if err != nil {
		t.Fatalf("apply metadata: %v", err)
	}
	if updated.Title != "Прей" {
		t.Fatalf("title = %q, want the canonical title kept", updated.Title)
	}
	found, ok := svc.LookupByTitle("Prey")
	if !ok || found.ID != game.ID {
		t.Fatalf("provider title was not remembered as an alias (%v)", ok)
	}
}

func TestApplyMetadataValidatesInput(t *testing.T) {
	svc, _ := metadataService(t)
	game, err := svc.AddGame(Game{Title: "Prey"})
	if err != nil {
		t.Fatalf("add game: %v", err)
	}

	cases := []struct {
		name   string
		gameID string
		patch  func(MetadataPatch) MetadataPatch
		want   error
	}{
		{
			name:   "unknown game",
			gameID: "нет-такой",
			patch:  func(p MetadataPatch) MetadataPatch { return p },
			want:   ErrNoGame,
		},
		{
			name:   "empty game",
			gameID: "  ",
			patch:  func(p MetadataPatch) MetadataPatch { return p },
			want:   ErrNoGame,
		},
		{
			name:   "no provider id",
			gameID: game.ID,
			patch:  func(p MetadataPatch) MetadataPatch { p.IGDBID = ""; return p },
			want:   errNoProvider,
		},
		{
			name:   "no timestamp",
			gameID: game.ID,
			patch:  func(p MetadataPatch) MetadataPatch { p.UpdatedAt = time.Time{}; return p },
			want:   errNoTimestamp,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := svc.ApplyMetadata(tc.gameID, tc.patch(samplePatch())); !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
			stored, err := svc.GetGame(game.ID)
			if err != nil {
				t.Fatalf("reload: %v", err)
			}
			if stored.ExternalIDs.IGDB != "" {
				t.Fatalf("game mutated on a rejected patch: %+v", stored)
			}
		})
	}
}

func TestApplyMetadataRollsBackOnWriteFailure(t *testing.T) {
	svc, dir := metadataService(t)
	game, err := svc.AddGame(Game{Title: "Prey", Provisional: true})
	if err != nil {
		t.Fatalf("add game: %v", err)
	}

	path := filepath.Join(dir, "catalog.json")
	if err := os.Remove(path); err != nil {
		t.Fatalf("remove catalog: %v", err)
	}
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatalf("block catalog path: %v", err)
	}

	if _, err := svc.ApplyMetadata(game.ID, samplePatch()); err == nil {
		t.Fatal("apply succeeded despite an unwritable catalog")
	}

	stored, err := svc.GetGame(game.ID)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if stored.ExternalIDs.IGDB != "" || stored.Summary != "" || stored.MetadataUpdatedAt != nil {
		t.Fatalf("in-memory game kept the failed patch: %+v", stored)
	}
	if !stored.Provisional {
		t.Fatal("provisional flag cleared despite the failed write")
	}
	if _, ok := svc.LookupByTitle("Prey"); !ok {
		t.Fatal("index broken after rollback")
	}
}

func TestApplyMetadataKeepsYearWhenProviderHasNoDate(t *testing.T) {
	svc, _ := metadataService(t)
	year := 2006
	game, err := svc.AddGame(Game{Title: "Prey", ReleaseYear: &year})
	if err != nil {
		t.Fatalf("add game: %v", err)
	}

	patch := samplePatch()
	patch.ReleaseDate = nil
	updated, err := svc.ApplyMetadata(game.ID, patch)
	if err != nil {
		t.Fatalf("apply metadata: %v", err)
	}
	if updated.ReleaseYear == nil || *updated.ReleaseYear != 2006 {
		t.Fatalf("release year = %v, want the existing one kept", updated.ReleaseYear)
	}
	if updated.ReleaseDate != nil {
		t.Fatalf("release date = %v, want nil", updated.ReleaseDate)
	}
}

func TestApplyMetadataResolvesByExternalID(t *testing.T) {
	svc, _ := metadataService(t)
	game, err := svc.AddGame(Game{Title: "Prey"})
	if err != nil {
		t.Fatalf("add game: %v", err)
	}
	if _, err := svc.ApplyMetadata(game.ID, samplePatch()); err != nil {
		t.Fatalf("apply metadata: %v", err)
	}

	match := svc.Resolve(Query{Title: "совсем другое имя", ExternalIDs: ExternalIDs{IGDB: "2657"}})
	if match.GameID != game.ID || match.Method != MethodExternalID {
		t.Fatalf("resolve = %+v", match)
	}
}

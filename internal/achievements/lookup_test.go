package achievements

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"

	"typhon/internal/catalog"
	"typhon/internal/library"
	"typhon/internal/metadata"
	"typhon/internal/metadata/typhonapi"
	"typhon/internal/uierr"
)

type fakeLibrary struct {
	games map[string]library.Game
	err   error
}

func (f fakeLibrary) Find(id string) (library.Game, error) {
	if f.err != nil {
		return library.Game{}, f.err
	}
	if g, ok := f.games[id]; ok {
		return g, nil
	}
	return library.Game{}, uierr.New("library.game_not_found", "игра не найдена")
}

type fakeCatalog struct{ games map[string]catalog.Game }

func (f fakeCatalog) GetGames(ids []string) []catalog.Game {
	var out []catalog.Game
	for _, id := range ids {
		if g, ok := f.games[id]; ok {
			out = append(out, g)
		}
	}
	return out
}

func TestLookup(t *testing.T) {
	libErr := errors.New("library is locked")
	lib := fakeLibrary{games: map[string]library.Game{
		"imported":  {InstallDir: `C:\Games\A`, External: &library.External{Provider: library.ProviderSteam, AppID: "620"}},
		"gog":       {InstallDir: `C:\Games\G`, External: &library.External{Provider: library.ProviderGOG, AppID: "1207"}},
		"matched":   {InstallDir: `C:\Games\B`, CanonicalGameID: "cat-1"},
		"local-id":  {InstallDir: `C:\Games\C`, CanonicalGameID: "cat-2"},
		"unmatched": {InstallDir: `C:\Games\D`, CanonicalGameID: "cat-404"},
		"plain":     {InstallDir: `C:\Games\E`},
		"bad-id":    {InstallDir: `C:\Games\F`, CanonicalGameID: "cat-3"},
	}}
	cat := fakeCatalog{games: map[string]catalog.Game{
		"cat-1": {ID: "cat-1", ExternalIDs: catalog.ExternalIDs{Steam: "400"}},
		"cat-2": {ID: "cat-2", LocalExternalIDs: catalog.ExternalIDs{Steam: "500"}},
		"cat-3": {ID: "cat-3", ExternalIDs: catalog.ExternalIDs{Steam: "../4"}},
	}}
	l, err := NewLookup(lib, cat)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		id    string
		ref   GameRef
		found bool
	}{
		{"imported", GameRef{InstallDir: `C:\Games\A`, AppID: "620"}, true},
		{"gog", GameRef{InstallDir: `C:\Games\G`}, true},
		{"matched", GameRef{InstallDir: `C:\Games\B`, AppID: "400"}, true},
		{"local-id", GameRef{InstallDir: `C:\Games\C`, AppID: "500"}, true},
		{"unmatched", GameRef{InstallDir: `C:\Games\D`}, true},
		{"plain", GameRef{InstallDir: `C:\Games\E`}, true},
		{"bad-id", GameRef{InstallDir: `C:\Games\F`}, true},
		{"missing", GameRef{}, false},
	}
	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			ref, found, err := l.Lookup(tc.id)
			if err != nil || found != tc.found || ref != tc.ref {
				t.Fatalf("ref %+v found %v err %v", ref, found, err)
			}
		})
	}
	failing, err := NewLookup(fakeLibrary{err: libErr}, cat)
	if err != nil {
		t.Fatal(err)
	}
	if _, found, err := failing.Lookup("imported"); found || !errors.Is(err, libErr) {
		t.Fatalf("found %v err %v: a library failure is not a missing game", found, err)
	}
	if _, err := NewLookup(nil, cat); err == nil {
		t.Fatal("nil library must be rejected")
	}
}

type fakeAPI struct {
	set  typhonapi.AchievementSet
	err  error
	lang string
}

func (f *fakeAPI) Achievements(ctx context.Context, _ string) (typhonapi.AchievementSet, error) {
	f.lang = metadata.Language(ctx)
	return f.set, f.err
}

func TestSchemaSourceMapsAPIErrors(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want error
		not  error
	}{
		{"no achievements", fmt.Errorf("%w: HTTP 404", typhonapi.ErrNoAchievements), ErrNoSchema, ErrSchemaDown},
		{"steam unavailable", fmt.Errorf("%w: HTTP 503", typhonapi.ErrSteamUnavailable), ErrSchemaDown, ErrNoSchema},
		{"other", typhonapi.ErrUpstream, typhonapi.ErrUpstream, ErrNoSchema},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src, err := NewSchemaSource(&fakeAPI{err: tc.err})
			if err != nil {
				t.Fatal(err)
			}
			_, err = src.Schema(context.Background(), "620", "ru")
			if !errors.Is(err, tc.want) || errors.Is(err, tc.not) {
				t.Fatalf("err = %v, want %v and not %v", err, tc.want, tc.not)
			}
		})
	}
}

func TestSchemaSourceCarriesLanguageAndItems(t *testing.T) {
	api := &fakeAPI{set: typhonapi.AchievementSet{AppID: "620", Items: []typhonapi.Achievement{{Key: "A", Name: "Alpha", Hidden: true, Icon: "https://x/i.jpg"}}}}
	src, err := NewSchemaSource(api)
	if err != nil {
		t.Fatal(err)
	}
	got, err := src.Schema(context.Background(), "620", "ru")
	if err != nil {
		t.Fatal(err)
	}
	if api.lang != "ru" {
		t.Fatalf("language = %q", api.lang)
	}
	if !slices.Equal(got, []SchemaItem{{Key: "A", Name: "Alpha", Hidden: true, Icon: "https://x/i.jpg"}}) {
		t.Fatalf("got %+v", got)
	}
	if _, err := NewSchemaSource(nil); err == nil {
		t.Fatal("nil client must be rejected")
	}
}

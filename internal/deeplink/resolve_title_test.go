package deeplink

import (
	"context"
	"errors"
	"testing"

	"typhon/internal/catalog"
)

func TestResolveTitleOverFakeCatalog(t *testing.T) {
	exact := catalog.Candidate{GameID: "g1", Method: catalog.MethodExactTitle}
	alias := catalog.Candidate{GameID: "g2", Method: catalog.MethodAlias}
	fuzzy := catalog.Candidate{GameID: "g3", Method: catalog.MethodFuzzy}
	cases := []struct {
		name  string
		match catalog.Match
		want  Resolution
	}{
		{"one exact title", catalog.Match{Status: catalog.StatusReview, Candidates: []catalog.Candidate{exact}}, found("g1")},
		{"one alias", catalog.Match{Status: catalog.StatusReview, Candidates: []catalog.Candidate{alias}}, found("g2")},
		{"fuzzy edition next to the exact game", catalog.Match{Status: catalog.StatusReview, Candidates: []catalog.Candidate{exact, fuzzy}}, found("g1")},
		{"user override", catalog.Match{Status: catalog.StatusMatched, GameID: "g9", Method: catalog.MethodOverride}, found("g9")},
		{"two exact titles", catalog.Match{Status: catalog.StatusReview, Candidates: []catalog.Candidate{exact, {GameID: "g4", Method: catalog.MethodExactTitle}}}, Resolution{Status: StatusAmbiguous}},
		{"exact title and alias of another game", catalog.Match{Status: catalog.StatusReview, Candidates: []catalog.Candidate{exact, alias}}, Resolution{Status: StatusAmbiguous}},
		{"only fuzzy", catalog.Match{Status: catalog.StatusReview, Candidates: []catalog.Candidate{fuzzy}}, Resolution{Status: StatusNotFound}},
		{"nothing", catalog.Match{Status: catalog.StatusUnmatched}, Resolution{Status: StatusNotFound}},
		{"candidate without id", catalog.Match{Candidates: []catalog.Candidate{{Method: catalog.MethodExactTitle}}}, Resolution{Status: StatusNotFound}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			games := &fakeCatalog{match: tc.match}
			lookup := &fakeMetadata{echo: true}
			got, err := newTestService(t, games, lookup).ResolveTitle(context.Background(), "Hades")
			if err != nil || got != tc.want {
				t.Fatalf("ResolveTitle = %+v, %v; want %+v", got, err, tc.want)
			}
			if len(lookup.asked) != 0 || len(games.ensured) != 0 {
				t.Fatalf("network asked %q, catalog ensured %q", lookup.asked, games.ensured)
			}
		})
	}
}

func TestResolveTitleOverRealCatalog(t *testing.T) {
	games, err := catalog.NewServiceAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	add := func(game catalog.Game) string {
		t.Helper()
		added, err := games.AddGame(game)
		if err != nil {
			t.Fatal(err)
		}
		return added.ID
	}
	witcher := add(catalog.Game{Title: "The Witcher 3: Wild Hunt"})
	add(catalog.Game{Title: "The Witcher 3: Wild Hunt - Game of the Year Edition"})
	baldur := add(catalog.Game{Title: "Baldur's Gate 3", Aliases: []string{"BG3"}})
	add(catalog.Game{Title: "Prey"})
	add(catalog.Game{Title: "Prey"})
	add(catalog.Game{Title: "Solo Addon", GameType: "dlc"})
	before := len(games.ListGames())

	cases := []struct {
		name, query string
		want        Resolution
	}{
		{"exact", "The Witcher 3: Wild Hunt", found(witcher)},
		{"case and punctuation", "the WITCHER 3 - wild hunt", found(witcher)},
		{"apostrophe", "BALDURS GATE 3", found(baldur)},
		{"alias", "bg3", found(baldur)},
		{"two games with one title", "Prey", Resolution{Status: StatusAmbiguous}},
		{"edition without an exact entry stays a search", "The Witcher 3: Wild Hunt Complete Edition", Resolution{Status: StatusNotFound}},
		{"dlc is not a game page", "Solo Addon", Resolution{Status: StatusNotFound}},
		{"no such game", "Zzz Unknown Title", Resolution{Status: StatusNotFound}},
		{"punctuation only", "!!!", Resolution{Status: StatusNotFound}},
	}
	for _, remote := range []bool{false, true} {
		if remote {
			games.SetRemoteCatalog(noRemote{})
		}
		for _, tc := range cases {
			name := tc.name
			if remote {
				name += " next to a remote catalog"
			}
			t.Run(name, func(t *testing.T) {
				lookup := &fakeMetadata{echo: true}
				got, err := newTestService(t, games, lookup).ResolveTitle(context.Background(), tc.query)
				if err != nil || got != tc.want {
					t.Fatalf("ResolveTitle(%q) = %+v, %v; want %+v", tc.query, got, err, tc.want)
				}
				if len(lookup.asked) != 0 {
					t.Fatalf("network asked %q", lookup.asked)
				}
			})
		}
	}
	if after := len(games.ListGames()); after != before {
		t.Fatalf("catalog grew from %d to %d", before, after)
	}
}

// noRemote panics on any call: a lookup that touches the remote catalog fails.
type noRemote struct{ catalog.RemoteCatalog }

func TestResolveTitleRejects(t *testing.T) {
	cases := []struct {
		name, query string
		want        error
	}{
		{"empty", "", ErrQueryEmpty},
		{"spaces", "   ", ErrQueryEmpty},
		{"control", "a\nb", ErrQueryControl},
		{"not utf-8", "a\xff", ErrQueryUTF8},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			games := &fakeCatalog{match: catalog.Match{Candidates: []catalog.Candidate{{GameID: "g1", Method: catalog.MethodExactTitle}}}}
			got, err := newTestService(t, games, &fakeMetadata{}).ResolveTitle(context.Background(), tc.query)
			if !errors.Is(err, tc.want) || got != (Resolution{}) {
				t.Fatalf("ResolveTitle = %+v, %v; want %v", got, err, tc.want)
			}
		})
	}
}

func TestResolveTitleHonoursCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	games := &fakeCatalog{match: catalog.Match{Candidates: []catalog.Candidate{{GameID: "g1", Method: catalog.MethodExactTitle}}}}
	got, err := newTestService(t, games, &fakeMetadata{}).ResolveTitle(ctx, "Hades")
	if !errors.Is(err, context.Canceled) || got != (Resolution{}) {
		t.Fatalf("ResolveTitle = %+v, %v", got, err)
	}
}

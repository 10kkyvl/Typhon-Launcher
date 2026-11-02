package deeplink

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"typhon/internal/catalog"
	"typhon/internal/metadata"
	"typhon/internal/metadata/typhonapi"
	"typhon/internal/uierr"
)

func TestResolveTitleLocalMatchAsksNoServer(t *testing.T) {
	exact := catalog.Candidate{GameID: "g1", Method: catalog.MethodExactTitle}
	alias := catalog.Candidate{GameID: "g2", Method: catalog.MethodAlias}
	fuzzy := catalog.Candidate{GameID: "g3", Method: catalog.MethodFuzzy}
	cases := []struct {
		name  string
		match catalog.Match
		want  string
	}{
		{"one exact title", catalog.Match{Status: catalog.StatusReview, Candidates: []catalog.Candidate{exact}}, "g1"},
		{"one alias", catalog.Match{Status: catalog.StatusReview, Candidates: []catalog.Candidate{alias}}, "g2"},
		{"fuzzy next to the exact game", catalog.Match{Status: catalog.StatusReview, Candidates: []catalog.Candidate{exact, fuzzy}}, "g1"},
		{"user override", catalog.Match{Status: catalog.StatusMatched, GameID: "g9", Method: catalog.MethodOverride}, "g9"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			lookup := &fakeMetadata{}
			got, err := newTestService(t, &fakeCatalog{match: tc.match}, lookup).ResolveTitle(context.Background(), "Hades")
			if err != nil || got != found(tc.want) {
				t.Fatalf("ResolveTitle = %+v, %v; want %q", got, err, tc.want)
			}
			if len(lookup.searched) != 0 {
				t.Fatalf("server asked %q", lookup.searched)
			}
		})
	}
}

func TestResolveTitleAsksTheServerAfterTheCatalog(t *testing.T) {
	twoLocal := catalog.Match{Candidates: []catalog.Candidate{
		{GameID: "g1", Method: catalog.MethodExactTitle},
		{GameID: "g2", Method: catalog.MethodExactTitle},
	}}
	noID := catalog.Match{Candidates: []catalog.Candidate{{Method: catalog.MethodExactTitle}}}
	onlyFuzzy := catalog.Match{Candidates: []catalog.Candidate{{GameID: "g3", Method: catalog.MethodFuzzy}}}
	igdb := metadata.Candidate{ProviderID: "1942", Title: "The Witcher 3: Wild Hunt"}
	cases := []struct {
		name       string
		local      catalog.Match
		candidates []metadata.Candidate
		searchErr  error
		want       Resolution
		ensured    int
	}{
		{"one exact opens a record", catalog.Match{}, []metadata.Candidate{igdb}, nil, found("new-1942"), 1},
		{"case and marks", catalog.Match{}, []metadata.Candidate{{ProviderID: "1942", Title: "THE WITCHER 3 - WILD HUNT"}}, nil, found("new-1942"), 1},
		{"edition next to the exact game", catalog.Match{}, []metadata.Candidate{igdb, {ProviderID: "7", Title: "The Witcher 3: Wild Hunt - Complete Edition"}}, nil, found("new-1942"), 1},
		{"only fuzzy locally", onlyFuzzy, []metadata.Candidate{igdb}, nil, found("new-1942"), 1},
		{"candidate without id locally", noID, []metadata.Candidate{igdb}, nil, found("new-1942"), 1},
		{"same game listed twice", catalog.Match{}, []metadata.Candidate{igdb, igdb}, nil, found("new-1942"), 1},
		{"two exact", catalog.Match{}, []metadata.Candidate{igdb, {ProviderID: "7", Title: "The Witcher 3: Wild Hunt"}}, nil, Resolution{Status: StatusAmbiguous}, 0},
		{"two local, server settles it", twoLocal, []metadata.Candidate{igdb}, nil, found("new-1942"), 1},
		{"two local, server knows none", twoLocal, nil, nil, Resolution{Status: StatusAmbiguous}, 0},
		{"none", catalog.Match{}, nil, nil, Resolution{Status: StatusNotFound}, 0},
		{"only inexact", catalog.Match{}, []metadata.Candidate{{ProviderID: "7", Title: "The Witcher 2"}}, nil, Resolution{Status: StatusNotFound}, 0},
		{"server says no match", catalog.Match{}, nil, fmt.Errorf("%w: 404", metadata.ErrNoMatch), Resolution{Status: StatusNotFound}, 0},
		{"steam only game", catalog.Match{}, []metadata.Candidate{{ProviderID: "steam:440", Title: "The Witcher 3: Wild Hunt"}}, nil, Resolution{Status: StatusNotInCatalog}, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			games := &fakeCatalog{match: tc.local}
			lookup := &fakeMetadata{candidates: tc.candidates, searchErr: tc.searchErr}
			got, err := newTestService(t, games, lookup).ResolveTitle(context.Background(), " The Witcher 3: Wild Hunt ")
			if err != nil || got != tc.want {
				t.Fatalf("ResolveTitle = %+v, %v; want %+v", got, err, tc.want)
			}
			if len(lookup.searched) != 1 || lookup.searched[0] != "The Witcher 3: Wild Hunt" {
				t.Fatalf("searched %q", lookup.searched)
			}
			if lookup.language != "ru" {
				t.Fatalf("language = %q", lookup.language)
			}
			if len(games.ensured) != tc.ensured {
				t.Fatalf("ensured %q, want %d records", games.ensured, tc.ensured)
			}
		})
	}
}

func TestResolveTitleOpensAnExistingRecordWithoutANewOne(t *testing.T) {
	games := &fakeCatalog{byIGDB: map[string]catalog.Game{"1942": {ID: "kept"}}}
	lookup := &fakeMetadata{candidates: []metadata.Candidate{{ProviderID: "1942", Title: "Hades"}}}
	got, err := newTestService(t, games, lookup).ResolveTitle(context.Background(), "Hades")
	if err != nil || got != found("kept") || len(games.ensured) != 0 {
		t.Fatalf("ResolveTitle = %+v, %v, ensured %q", got, err, games.ensured)
	}
}

func TestResolveTitleSkipsTheServerForATitleWithNoLettersOrDigits(t *testing.T) {
	lookup := &fakeMetadata{}
	got, err := newTestService(t, &fakeCatalog{}, lookup).ResolveTitle(context.Background(), "!!!")
	if err != nil || got.Status != StatusNotFound || len(lookup.searched) != 0 {
		t.Fatalf("ResolveTitle = %+v, %v, searched %q", got, err, lookup.searched)
	}
}

func TestResolveTitleErrors(t *testing.T) {
	transport := &url.Error{Op: "Get", URL: "https://api.example.com/metadata/search?q=secret-payload", Err: errors.New("dial failed")}
	cases := []struct {
		name string
		err  error
		code string
	}{
		{"network", fmt.Errorf("%w: %w", typhonapi.ErrUpstream, transport), codeUnavailable},
		{"unknown failure", transport, codeUnavailable},
		{"outdated", fmt.Errorf("%w: 426 %w", typhonapi.ErrOutdated, transport), codeOutdated},
		{"bad request", fmt.Errorf("%w: 400 %w", typhonapi.ErrBadRequest, transport), codeBadRequest},
		{"rate limit", &metadata.RateLimitError{}, "metadata.rate_limited"},
		{"not configured", metadata.ErrNotConfigured, "metadata.not_configured"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			games := &fakeCatalog{}
			lookup := &fakeMetadata{searchErr: tc.err}
			got, err := newTestService(t, games, lookup).ResolveTitle(context.Background(), "Hades")
			if got != (Resolution{}) || uierr.Code(err) != tc.code {
				t.Fatalf("ResolveTitle = %+v, %v; want code %q", got, err, tc.code)
			}
			if strings.Contains(err.Error(), "secret-payload") {
				t.Fatalf("error text carries the title: %q", err)
			}
			if len(games.ensured) != 0 {
				t.Fatalf("ensured %q", games.ensured)
			}
		})
	}
}

func TestResolveTitleRejectsABadProviderID(t *testing.T) {
	for _, id := range []string{"abc", "steam:abc", "0"} {
		lookup := &fakeMetadata{candidates: []metadata.Candidate{{ProviderID: id, Title: "Hades"}}}
		games := &fakeCatalog{}
		got, err := newTestService(t, games, lookup).ResolveTitle(context.Background(), "Hades")
		if !errors.Is(err, ErrBadProviderID) || got != (Resolution{}) || len(games.ensured) != 0 {
			t.Fatalf("id %q: %+v, %v, ensured %q", id, got, err, games.ensured)
		}
	}
}

func TestResolveTitleSharesTheNewGameLimit(t *testing.T) {
	games := &fakeCatalog{}
	lookup := &fakeMetadata{}
	s := newTestService(t, games, lookup)
	for i := range maxNewGames + 1 {
		lookup.candidates = []metadata.Candidate{{ProviderID: strconv.Itoa(i + 1), Title: "Hades"}}
		got, err := s.ResolveTitle(context.Background(), "Hades")
		if i < maxNewGames {
			if err != nil || got.Status != StatusFound {
				t.Fatalf("link %d: %+v, %v", i, got, err)
			}
			continue
		}
		if !errors.Is(err, ErrTooManyNewGames) || len(games.ensured) != maxNewGames {
			t.Fatalf("over the limit: %+v, %v, ensured %d", got, err, len(games.ensured))
		}
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
		searched    int
	}{
		{"exact", "The Witcher 3: Wild Hunt", found(witcher), 0},
		{"case and punctuation", "the WITCHER 3 - wild hunt", found(witcher), 0},
		{"apostrophe", "BALDURS GATE 3", found(baldur), 0},
		{"alias", "bg3", found(baldur), 0},
		{"two games with one title", "Prey", Resolution{Status: StatusAmbiguous}, 1},
		{"edition without an exact entry", "The Witcher 3: Wild Hunt Complete Edition", Resolution{Status: StatusNotFound}, 1},
		{"dlc is not a game page", "Solo Addon", Resolution{Status: StatusNotFound}, 1},
		{"no such game", "Zzz Unknown Title", Resolution{Status: StatusNotFound}, 1},
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
				lookup := &fakeMetadata{}
				got, err := newTestService(t, games, lookup).ResolveTitle(context.Background(), tc.query)
				if err != nil || got != tc.want {
					t.Fatalf("ResolveTitle(%q) = %+v, %v; want %+v", tc.query, got, err, tc.want)
				}
				if len(lookup.searched) != tc.searched {
					t.Fatalf("searched %q, want %d", lookup.searched, tc.searched)
				}
			})
		}
	}
	if after := len(games.ListGames()); after != before {
		t.Fatalf("catalog grew from %d to %d", before, after)
	}
}

// noRemote panics on any call: a local lookup that touches it fails the test.
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
			lookup := &fakeMetadata{}
			got, err := newTestService(t, &fakeCatalog{}, lookup).ResolveTitle(context.Background(), tc.query)
			if !errors.Is(err, tc.want) || got != (Resolution{}) || len(lookup.searched) != 0 {
				t.Fatalf("ResolveTitle = %+v, %v, searched %q; want %v", got, err, lookup.searched, tc.want)
			}
		})
	}
}

func TestResolveTitleHonoursCancellation(t *testing.T) {
	igdb := []metadata.Candidate{{ProviderID: "1942", Title: "Hades"}}
	t.Run("before the lookup", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		lookup := &fakeMetadata{candidates: igdb}
		got, err := newTestService(t, &fakeCatalog{}, lookup).ResolveTitle(ctx, "Hades")
		if !errors.Is(err, context.Canceled) || got != (Resolution{}) || len(lookup.searched) != 0 {
			t.Fatalf("ResolveTitle = %+v, %v, searched %q", got, err, lookup.searched)
		}
	})
	t.Run("during the search no record is written", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		games := &fakeCatalog{}
		lookup := &fakeMetadata{candidates: igdb, onSearch: cancel}
		got, err := newTestService(t, games, lookup).ResolveTitle(ctx, "Hades")
		if !errors.Is(err, context.Canceled) || got != (Resolution{}) || len(games.ensured) != 0 {
			t.Fatalf("ResolveTitle = %+v, %v, ensured %q", got, err, games.ensured)
		}
	})
	t.Run("a failure after cancel reads as cancel", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		lookup := &fakeMetadata{searchErr: typhonapi.ErrUpstream, onSearch: cancel}
		_, err := newTestService(t, &fakeCatalog{}, lookup).ResolveTitle(ctx, "Hades")
		if !errors.Is(err, context.Canceled) || uierr.Code(err) != "" {
			t.Fatalf("error = %v", err)
		}
	})
}

package deeplink

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"typhon/internal/catalog"
	"typhon/internal/metadata"
	"typhon/internal/metadata/typhonapi"
	"typhon/internal/uierr"
)

// spacedService moves the clock past the search gap on every read.
func spacedService(t *testing.T, games Catalog, lookup Metadata) *Service {
	t.Helper()
	s := newTestService(t, games, lookup)
	clock := time.Unix(1000, 0)
	s.now = func() time.Time {
		clock = clock.Add(minTitleSearchGap)
		return clock
	}
	return s
}

func TestResolveTitleAsksTheServerFirst(t *testing.T) {
	const name = "The Witcher 3: Wild Hunt"
	igdb := metadata.Candidate{ProviderID: "1942", Title: name, GameType: "Main Game"}
	localHit := catalog.Match{Status: catalog.StatusMatched, GameID: "cached", Method: catalog.MethodOverride, Candidates: []catalog.Candidate{{GameID: "cached", Method: catalog.MethodExactTitle}}}
	notFound := Resolution{Status: StatusNotFound}
	ambiguous := Resolution{Status: StatusAmbiguous}
	cases := []struct {
		name       string
		candidates []metadata.Candidate
		searchErr  error
		want       Resolution
		ensured    int
	}{
		{"one exact opens a record", []metadata.Candidate{igdb}, nil, found("new-1942"), 1},
		{"old server without a type", []metadata.Candidate{{ProviderID: "1942", Title: name}}, nil, found("new-1942"), 1},
		{"case and marks", []metadata.Candidate{{ProviderID: "1942", Title: "THE WITCHER 3 - WILD HUNT"}}, nil, found("new-1942"), 1},
		{"other edition title next to the game", []metadata.Candidate{igdb, {ProviderID: "7", Title: name + " - Complete Edition", GameType: "Main Game"}}, nil, found("new-1942"), 1},
		{"same game listed twice", []metadata.Candidate{igdb, igdb}, nil, found("new-1942"), 1},
		{"bundle edition is a game", []metadata.Candidate{{ProviderID: "1942", Title: name, GameType: "Bundle"}}, nil, found("new-1942"), 1},
		{"edition under its parent is one game", []metadata.Candidate{igdb, {ProviderID: "7", Title: name, GameType: "Bundle", ParentID: "1942"}}, nil, found("new-1942"), 1},
		{"dlc is dropped", []metadata.Candidate{{ProviderID: "7", Title: name, GameType: "DLC"}}, nil, notFound, 0},
		{"expansion is dropped", []metadata.Candidate{{ProviderID: "7", Title: name, GameType: "Expansion"}}, nil, notFound, 0},
		{"update is dropped", []metadata.Candidate{{ProviderID: "7", Title: name, GameType: "Update", ParentID: "1942"}}, nil, notFound, 0},
		{"dlc next to the game", []metadata.Candidate{igdb, {ProviderID: "7", Title: name, GameType: "DLC"}}, nil, found("new-1942"), 1},
		{"two exact", []metadata.Candidate{igdb, {ProviderID: "7", Title: name, GameType: "Remake"}}, nil, ambiguous, 0},
		{"none", nil, nil, notFound, 0},
		{"only inexact", []metadata.Candidate{{ProviderID: "7", Title: "The Witcher 2"}}, nil, notFound, 0},
		{"server says no match", nil, fmt.Errorf("%w: 404", metadata.ErrNoMatch), notFound, 0},
		{"metadata not configured", nil, metadata.ErrNotConfigured, notFound, 0},
		{"steam only game", []metadata.Candidate{{ProviderID: "steam:440", Title: name}}, nil, Resolution{Status: StatusNotInCatalog}, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			games := &fakeCatalog{match: localHit}
			lookup := &fakeMetadata{candidates: tc.candidates, searchErr: tc.searchErr}
			got, err := newTestService(t, games, lookup).ResolveTitle(context.Background(), " "+name+" ")
			if err != nil || got != tc.want {
				t.Fatalf("ResolveTitle = %+v, %v; want %+v", got, err, tc.want)
			}
			if len(lookup.searched) != 1 || lookup.searched[0] != name {
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

func TestResolveTitleSearchLimit(t *testing.T) {
	start := time.Unix(1000, 0)
	cases := []struct {
		name  string
		gaps  []time.Duration
		allow []bool
	}{
		{"first is free", []time.Duration{0}, []bool{true}},
		{"just under the gap", []time.Duration{0, minTitleSearchGap - time.Nanosecond}, []bool{true, false}},
		{"exactly the gap", []time.Duration{0, minTitleSearchGap}, []bool{true, true}},
		{"a refused attempt does not move the window", []time.Duration{0, time.Second, minTitleSearchGap - time.Second}, []bool{true, false, true}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			lookup := &fakeMetadata{}
			s := newTestService(t, &fakeCatalog{}, lookup)
			now := start
			s.now = func() time.Time { return now }
			asked := 0
			for i, gap := range tc.gaps {
				now = now.Add(gap)
				got, err := s.ResolveTitle(context.Background(), "Hades")
				if tc.allow[i] {
					asked++
					if err != nil || got.Status != StatusNotFound {
						t.Fatalf("attempt %d: %+v, %v", i, got, err)
					}
				} else if !errors.Is(err, ErrTooManyTitleSearches) || uierr.Code(err) != "link.too_many" {
					t.Fatalf("attempt %d: %+v, %v", i, got, err)
				}
				if len(lookup.searched) != asked {
					t.Fatalf("attempt %d: asked %d, want %d", i, len(lookup.searched), asked)
				}
			}
		})
	}
}

func TestResolveTitleStopsAfterTheSessionLimit(t *testing.T) {
	lookup := &fakeMetadata{}
	s := spacedService(t, &fakeCatalog{}, lookup)
	for i := range maxTitleSearches {
		if _, err := s.ResolveTitle(context.Background(), "Hades"); err != nil {
			t.Fatalf("search %d: %v", i+1, err)
		}
	}
	_, err := s.ResolveTitle(context.Background(), "Hades")
	if !errors.Is(err, ErrTooManyTitleSearches) || len(lookup.searched) != maxTitleSearches {
		t.Fatalf("over the limit: %v, asked %d", err, len(lookup.searched))
	}
}

func TestResolveTitleCountsTheSearchBeforeTheRequest(t *testing.T) {
	lookup := &fakeMetadata{searchErr: typhonapi.ErrUpstream}
	s := newTestService(t, &fakeCatalog{}, lookup)
	lookup.onSearch = func() {
		if s.searches != 1 {
			t.Errorf("searches = %d during the request", s.searches)
		}
	}
	if _, err := s.ResolveTitle(context.Background(), "Hades"); uierr.Code(err) != codeUnavailable {
		t.Fatalf("error = %v", err)
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
		{"outdated", fmt.Errorf("%w: 426 %w", typhonapi.ErrOutdated, transport), codeOutdated},
		{"bad request", fmt.Errorf("%w: 400 %w", typhonapi.ErrBadRequest, transport), codeBadRequest},
		{"rate limit", &metadata.RateLimitError{}, "metadata.rate_limited"},
		{"unknown failure has no code", transport, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var logged bytes.Buffer
			previous := slog.Default()
			slog.SetDefault(slog.New(slog.NewTextHandler(&logged, nil)))
			t.Cleanup(func() { slog.SetDefault(previous) })

			games := &fakeCatalog{}
			lookup := &fakeMetadata{searchErr: tc.err}
			got, err := newTestService(t, games, lookup).ResolveTitle(context.Background(), "Hades")
			if err == nil || got != (Resolution{}) || uierr.Code(err) != tc.code {
				t.Fatalf("ResolveTitle = %+v, %v; want code %q", got, err, tc.code)
			}
			if strings.Contains(err.Error(), "secret-payload") || strings.Contains(logged.String(), "secret-payload") || strings.Contains(logged.String(), "Hades") {
				t.Fatalf("error %q or log %q carries the text", err, logged.String())
			}
			if tc.code == "" && !strings.Contains(logged.String(), "errorType=*url.Error") {
				t.Fatalf("log = %q", logged.String())
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
	s := spacedService(t, games, lookup)
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

func TestResolveTitleRejects(t *testing.T) {
	cases := []struct {
		name, query string
		want        error
	}{
		{"empty", "", ErrQueryEmpty},
		{"spaces", "   ", ErrQueryEmpty},
		{"control", "a\nb", ErrQueryControl},
		{"not utf-8", "a\xff", ErrQueryUTF8},
		{"over the byte limit", strings.Repeat("я", 101), ErrQueryTooLong},
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
		s := newTestService(t, &fakeCatalog{}, lookup)
		got, err := s.ResolveTitle(ctx, "Hades")
		if !errors.Is(err, context.Canceled) || got != (Resolution{}) || len(lookup.searched) != 0 || s.searches != 0 {
			t.Fatalf("ResolveTitle = %+v, %v, searched %q, counted %d", got, err, lookup.searched, s.searches)
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

package deeplink

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"testing"

	"typhon/internal/catalog"
	"typhon/internal/metadata"
	"typhon/internal/metadata/typhonapi"
	"typhon/internal/uierr"
)

type fakeCatalog struct {
	byIGDB    map[string]catalog.Game
	match     catalog.Match
	ensureErr error
	ensured   []string
}

func (c *fakeCatalog) GameByIGDB(id string) (catalog.Game, bool) {
	game, ok := c.byIGDB[id]
	return game, ok
}

func (c *fakeCatalog) Resolve(catalog.Query) catalog.Match {
	return c.match
}

func (c *fakeCatalog) EnsureByIGDB(id, title string) (catalog.Game, error) {
	c.ensured = append(c.ensured, id+"|"+title)
	if c.ensureErr != nil {
		return catalog.Game{}, c.ensureErr
	}
	return catalog.Game{ID: "new-" + id, Title: title}, nil
}

type fakeMetadata struct {
	meta     metadata.GameMetadata
	echo     bool
	err      error
	asked    []string
	language string
	cancel   context.CancelFunc
}

func (m *fakeMetadata) FetchGame(ctx context.Context, providerID string) (metadata.GameMetadata, error) {
	m.asked = append(m.asked, providerID)
	m.language = metadata.Language(ctx)
	if m.cancel != nil {
		m.cancel()
		if m.err == nil {
			return m.meta, nil
		}
		return metadata.GameMetadata{}, ctx.Err()
	}
	if m.echo {
		return metadata.GameMetadata{ProviderID: providerID, Title: "Game"}, nil
	}
	return m.meta, m.err
}

func newTestService(t *testing.T, games Catalog, lookup Metadata) *Service {
	t.Helper()
	s, err := NewService(games, lookup, func() string { return "ru" })
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestResolveOutcomes(t *testing.T) {
	network := fmt.Errorf("%w: connection refused", typhonapi.ErrUpstream)
	cases := []struct {
		name    string
		target  Target
		games   *fakeCatalog
		lookup  *fakeMetadata
		want    Resolution
		asked   string
		ensured string
	}{
		{
			name:   "igdb already in the catalog needs no network",
			target: Target{ProviderIGDB, "7"},
			games:  &fakeCatalog{byIGDB: map[string]catalog.Game{"7": {ID: "g7"}}},
			lookup: &fakeMetadata{err: network},
			want:   Resolution{StatusFound, "g7"},
		},
		{
			name:   "steam already in the catalog needs no network",
			target: Target{ProviderSteam, "440"},
			games:  &fakeCatalog{match: catalog.Match{Status: catalog.StatusMatched, GameID: "g440"}},
			lookup: &fakeMetadata{err: network},
			want:   Resolution{StatusFound, "g440"},
		},
		{
			name:    "igdb new to the catalog is opened with the title from the answer",
			target:  Target{ProviderIGDB, "7"},
			games:   &fakeCatalog{},
			lookup:  &fakeMetadata{meta: metadata.GameMetadata{ProviderID: "7", Title: "Seven"}},
			want:    Resolution{StatusFound, "new-7"},
			asked:   "7",
			ensured: "7|Seven",
		},
		{
			name:    "steam app linked to igdb is opened by its igdb id",
			target:  Target{ProviderSteam, "440"},
			games:   &fakeCatalog{match: catalog.Match{Status: catalog.StatusUnmatched}},
			lookup:  &fakeMetadata{meta: metadata.GameMetadata{ProviderID: "1942", SteamAppID: "440", Title: "Fortress"}},
			want:    Resolution{StatusFound, "new-1942"},
			asked:   "steam:440",
			ensured: "1942|Fortress",
		},
		{
			name:   "steam app whose igdb game is already held creates nothing",
			target: Target{ProviderSteam, "440"},
			games:  &fakeCatalog{byIGDB: map[string]catalog.Game{"1942": {ID: "g1942"}}},
			lookup: &fakeMetadata{meta: metadata.GameMetadata{ProviderID: "1942", Title: "Fortress"}},
			want:   Resolution{StatusFound, "g1942"},
			asked:  "steam:440",
		},
		{
			name:   "steam app without igdb is known but cannot enter the catalog",
			target: Target{ProviderSteam, "440"},
			games:  &fakeCatalog{},
			lookup: &fakeMetadata{meta: metadata.GameMetadata{ProviderID: "steam:440", SteamAppID: "440", Title: "Fortress"}},
			want:   Resolution{Status: StatusNotInCatalog},
			asked:  "steam:440",
		},
		{
			name:   "unknown to the provider is not found",
			target: Target{ProviderIGDB, "7"},
			games:  &fakeCatalog{},
			lookup: &fakeMetadata{err: fmt.Errorf("%w: 404", metadata.ErrNoMatch)},
			want:   Resolution{Status: StatusNotFound},
			asked:  "7",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := newTestService(t, tc.games, tc.lookup).Resolve(t.Context(), tc.target)
			if err != nil || got != tc.want {
				t.Fatalf("Resolve = %+v, %v; want %+v", got, err, tc.want)
			}
			var asked, ensured string
			if len(tc.lookup.asked) > 0 {
				asked = tc.lookup.asked[0]
			}
			if len(tc.games.ensured) > 0 {
				ensured = tc.games.ensured[0]
			}
			if asked != tc.asked || ensured != tc.ensured {
				t.Fatalf("asked %q, ensured %q; want %q, %q", asked, ensured, tc.asked, tc.ensured)
			}
		})
	}
}

func TestResolveErrorCodes(t *testing.T) {
	saveFailed := uierr.Wrap("catalog.save_failed", errors.New("disk full"))
	token := errors.New("получить токен сессии: keyring locked")
	cases := []struct {
		name   string
		lookup error
		ensure error
		code   string
		is     error
	}{
		{name: "network", lookup: fmt.Errorf("%w: connection refused", typhonapi.ErrUpstream), code: "link.unavailable", is: typhonapi.ErrUpstream},
		{name: "rate limit keeps its code", lookup: &metadata.RateLimitError{}, code: "metadata.rate_limited", is: metadata.ErrRateLimited},
		{name: "not configured keeps its code", lookup: metadata.ErrNotConfigured, code: "metadata.not_configured", is: metadata.ErrNotConfigured},
		{name: "outdated launcher", lookup: fmt.Errorf("%w: 426", typhonapi.ErrOutdated), code: "link.outdated", is: typhonapi.ErrOutdated},
		{name: "bad request", lookup: fmt.Errorf("%w: 400", typhonapi.ErrBadRequest), code: "link.bad_request", is: typhonapi.ErrBadRequest},
		{name: "a local failure is not called the network", lookup: token, code: "", is: token},
		{name: "catalog save keeps its code", ensure: saveFailed, code: "catalog.save_failed", is: saveFailed},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			games := &fakeCatalog{ensureErr: tc.ensure}
			lookup := &fakeMetadata{meta: metadata.GameMetadata{ProviderID: "7", Title: "Seven"}, err: tc.lookup}
			got, err := newTestService(t, games, lookup).Resolve(t.Context(), Target{ProviderIGDB, "7"})
			if !errors.Is(err, tc.is) || uierr.Code(err) != tc.code {
				t.Fatalf("error = %v, code %q; want code %q", err, uierr.Code(err), tc.code)
			}
			if got != (Resolution{}) {
				t.Fatalf("Resolve = %+v alongside an error", got)
			}
		})
	}
}

func TestResolveRejectsGarbageProviderID(t *testing.T) {
	for _, id := range []string{"", "abc", "0", "steam:", "steam:abc", "../7", "1234567890123"} {
		t.Run(id, func(t *testing.T) {
			games := &fakeCatalog{}
			lookup := &fakeMetadata{meta: metadata.GameMetadata{ProviderID: id, Title: "Seven"}}
			got, err := newTestService(t, games, lookup).Resolve(t.Context(), Target{ProviderIGDB, "7"})
			if !errors.Is(err, ErrBadProviderID) || uierr.Code(err) != "link.bad_response" {
				t.Fatalf("Resolve = %+v, %v", got, err)
			}
			if len(games.ensured) != 0 {
				t.Fatalf("created %v from a garbage id", games.ensured)
			}
		})
	}
}

func TestResolveNewGameLimit(t *testing.T) {
	games := &fakeCatalog{byIGDB: map[string]catalog.Game{"999": {ID: "known"}}}
	lookup := &fakeMetadata{echo: true}
	s := newTestService(t, games, lookup)

	for i := 1; i <= maxNewGames; i++ {
		got, err := s.Resolve(t.Context(), Target{ProviderIGDB, strconv.Itoa(i)})
		if err != nil || got.Status != StatusFound {
			t.Fatalf("link %d: %+v, %v", i, got, err)
		}
		// A game the catalog holds, asked for between new ones, spends nothing.
		if got, err := s.Resolve(t.Context(), Target{ProviderIGDB, "999"}); err != nil || got.GameID != "known" {
			t.Fatalf("known game: %+v, %v", got, err)
		}
	}
	created := len(games.ensured)
	if created != maxNewGames {
		t.Fatalf("created %d, want %d", created, maxNewGames)
	}

	_, err := s.Resolve(t.Context(), Target{ProviderIGDB, "5000"})
	if !errors.Is(err, ErrTooManyNewGames) || uierr.Code(err) != "link.too_many" {
		t.Fatalf("over the limit: %v", err)
	}
	if len(games.ensured) != created {
		t.Fatal("a record was created over the limit")
	}
	if got, err := s.Resolve(t.Context(), Target{ProviderIGDB, "999"}); err != nil || got.GameID != "known" {
		t.Fatalf("known game over the limit: %+v, %v", got, err)
	}
}

func TestResolveKnownSteamGameOverTheLimit(t *testing.T) {
	games := &fakeCatalog{byIGDB: map[string]catalog.Game{"7": {ID: "g7"}}}
	s := newTestService(t, games, &fakeMetadata{meta: metadata.GameMetadata{ProviderID: "7", Title: "Seven"}})
	s.created = maxNewGames
	got, err := s.Resolve(t.Context(), Target{ProviderSteam, "440"})
	if err != nil || got != (Resolution{StatusFound, "g7"}) {
		t.Fatalf("Resolve = %+v, %v", got, err)
	}
}

func TestResolveFailedWriteDoesNotSpendTheLimit(t *testing.T) {
	games := &fakeCatalog{ensureErr: errors.New("disk full")}
	s := newTestService(t, games, &fakeMetadata{echo: true})
	if _, err := s.Resolve(t.Context(), Target{ProviderIGDB, "7"}); err == nil {
		t.Fatal("want an error")
	}
	if s.created != 0 {
		t.Fatalf("created = %d", s.created)
	}
}

func TestResolveAsksInTheUILanguage(t *testing.T) {
	lookup := &fakeMetadata{err: metadata.ErrNoMatch}
	if _, err := newTestService(t, &fakeCatalog{}, lookup).Resolve(t.Context(), Target{ProviderIGDB, "7"}); err != nil {
		t.Fatal(err)
	}
	if lookup.language != "ru" {
		t.Fatalf("language = %q", lookup.language)
	}
}

func TestResolveCancelled(t *testing.T) {
	t.Run("before the request", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		lookup := &fakeMetadata{}
		_, err := newTestService(t, &fakeCatalog{}, lookup).Resolve(ctx, Target{ProviderIGDB, "7"})
		if !errors.Is(err, context.Canceled) || len(lookup.asked) != 0 {
			t.Fatalf("error = %v, asked %v", err, lookup.asked)
		}
	})
	t.Run("during the request is a cancel, not a failure", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		lookup := &fakeMetadata{cancel: cancel, err: errors.New("aborted")}
		_, err := newTestService(t, &fakeCatalog{}, lookup).Resolve(ctx, Target{ProviderIGDB, "7"})
		if !errors.Is(err, context.Canceled) || uierr.Code(err) != "" {
			t.Fatalf("error = %v, code %q", err, uierr.Code(err))
		}
	})
	t.Run("after the answer creates no record", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		games := &fakeCatalog{}
		lookup := &fakeMetadata{cancel: cancel, meta: metadata.GameMetadata{ProviderID: "7", Title: "Seven"}}
		s := newTestService(t, games, lookup)
		_, err := s.Resolve(ctx, Target{ProviderIGDB, "7"})
		if !errors.Is(err, context.Canceled) || len(games.ensured) != 0 || s.created != 0 {
			t.Fatalf("error = %v, ensured %v, created %d", err, games.ensured, s.created)
		}
	})
}

func TestResolveRevalidatesTheTarget(t *testing.T) {
	cases := []struct {
		name   string
		target Target
		want   error
	}{
		{"unknown provider", Target{"gog", "1"}, ErrProvider},
		{"empty", Target{}, ErrProvider},
		{"traversal", Target{ProviderIGDB, "../1"}, ErrID},
		{"zero", Target{ProviderSteam, "0"}, ErrZeroID},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			lookup := &fakeMetadata{}
			_, err := newTestService(t, &fakeCatalog{}, lookup).Resolve(t.Context(), tc.target)
			if !errors.Is(err, tc.want) || len(lookup.asked) != 0 {
				t.Fatalf("error = %v, asked %v", err, lookup.asked)
			}
		})
	}
}

func TestNewServiceNeedsEveryDependency(t *testing.T) {
	language := func() string { return "en" }
	if _, err := NewService(nil, &fakeMetadata{}, language); err == nil {
		t.Fatal("a service without a catalog must not start")
	}
	if _, err := NewService(&fakeCatalog{}, nil, language); err == nil {
		t.Fatal("a service without metadata must not start")
	}
	if _, err := NewService(&fakeCatalog{}, &fakeMetadata{}, nil); err == nil {
		t.Fatal("a service without a language source must not start")
	}
}

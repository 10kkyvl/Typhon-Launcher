package deeplink

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"typhon/internal/catalog"
	"typhon/internal/metadata"
	"typhon/internal/metadata/typhonapi"
	"typhon/internal/titles"
	"typhon/internal/uierr"
)

const (
	StatusFound        = "found"
	StatusNotFound     = "not_found"
	StatusNotInCatalog = "not_in_catalog"
	StatusAmbiguous    = "ambiguous"

	codeUnavailable = "link.unavailable"
	codeOutdated    = "link.outdated"
	codeBadRequest  = "link.bad_request"

	steamProviderPrefix = "steam:"

	// A page can open links in a loop, and every new game is a permanent record
	// in the user's catalog file.
	maxNewGames = 20

	// Every search link is a request to the metadata service with text a page
	// chose, so a page must not be able to drive the launcher's budget.
	minTitleSearchGap = 3 * time.Second
	maxTitleSearches  = 60

	gameTypeBundle = "Bundle"
)

var (
	ErrTooManyNewGames      = uierr.New("link.too_many", "по ссылкам добавлено слишком много новых игр, перезапустите лаунчер")
	ErrTooManyTitleSearches = uierr.New("link.too_many", "слишком много поисков по ссылкам, попробуйте позже")
	ErrBadProviderID        = uierr.New("link.bad_response", "сервер метаданных вернул некорректный идентификатор игры")

	errTitleLookup  = errors.New("не удалось найти игру по названию")
	errNoDependency = errors.New("для ссылок нужны каталог, метаданные и язык")
)

// Resolution always says which outcome it is, so a page never has to read an
// empty GameID as "no such game". A failed lookup is an error instead: the game
// may exist, and the user can try the link again.
type Resolution struct {
	Status string `json:"status"`
	GameID string `json:"gameId"`
}

//wails:ignore
type Catalog interface {
	GameByIGDB(igdbID string) (catalog.Game, bool)
	Resolve(q catalog.Query) catalog.Match
	EnsureByIGDB(igdbID, title string) (catalog.Game, error)
}

//wails:ignore
type Metadata interface {
	FetchGame(ctx context.Context, providerID string) (metadata.GameMetadata, error)
	SearchTitle(ctx context.Context, title string) ([]metadata.Candidate, error)
}

type Service struct {
	catalog  Catalog
	metadata Metadata
	language func() string

	now func() time.Time

	mu         sync.Mutex
	created    int
	searches   int
	lastSearch time.Time
}

func NewService(games Catalog, lookup Metadata, language func() string) (*Service, error) {
	if games == nil || lookup == nil || language == nil {
		return nil, errNoDependency
	}
	return &Service{catalog: games, metadata: lookup, language: language, now: time.Now}, nil
}

// Resolve finds the catalog game a link names. The target comes back from the
// page, so it is validated again rather than trusted.
func (s *Service) Resolve(ctx context.Context, target Target) (Resolution, error) {
	if err := ctx.Err(); err != nil {
		return Resolution{}, err
	}
	if err := target.Validate(); err != nil {
		return Resolution{}, err
	}
	if id, ok := s.localGame(target); ok {
		return found(id), nil
	}
	meta, err := s.metadata.FetchGame(metadata.WithLanguage(ctx, s.language()), target.providerID())
	if errors.Is(err, metadata.ErrNoMatch) {
		return Resolution{Status: StatusNotFound}, nil
	}
	if err != nil {
		if ctx.Err() != nil {
			return Resolution{}, ctx.Err()
		}
		return Resolution{}, lookupError(err)
	}
	providerID := strings.TrimSpace(meta.ProviderID)
	// The API answers a Steam app it has no IGDB record for under "steam:<id>",
	// and the catalog can only open a new game by its IGDB id.
	if steamID, steam := strings.CutPrefix(providerID, steamProviderPrefix); steam {
		if err := validateID(steamID); err != nil {
			return Resolution{}, fmt.Errorf("%w: %w", ErrBadProviderID, err)
		}
		return Resolution{Status: StatusNotInCatalog}, nil
	}
	if err := validateID(providerID); err != nil {
		return Resolution{}, fmt.Errorf("%w: %w", ErrBadProviderID, err)
	}
	return s.open(ctx, providerID, meta.Title)
}

// ResolveTitle finds the one game a search link names by asking the metadata
// service for exactly this title: the local catalog is a cache of visited pages
// and cannot show that a title is unique. Only the page calls it, and the page
// waits for the accepted terms and a session before taking a link. A catalog
// record appears only for a single exact game, under the same limit as a game
// link.
func (s *Service) ResolveTitle(ctx context.Context, query string) (Resolution, error) {
	if err := ctx.Err(); err != nil {
		return Resolution{}, err
	}
	title, err := validateQuery(query)
	if err != nil {
		return Resolution{}, err
	}
	normalized := titles.Normalize(title)
	if normalized == "" {
		return Resolution{Status: StatusNotFound}, nil
	}
	if err := s.takeSearch(); err != nil {
		return Resolution{}, err
	}
	candidates, err := s.metadata.SearchTitle(metadata.WithLanguage(ctx, s.language()), title)
	if errors.Is(err, metadata.ErrNoMatch) || errors.Is(err, metadata.ErrNotConfigured) {
		return Resolution{Status: StatusNotFound}, nil
	}
	if err != nil {
		if ctx.Err() != nil {
			return Resolution{}, ctx.Err()
		}
		return Resolution{}, titleLookupError(err)
	}
	exact := mainCandidates(candidates, normalized)
	switch len(exact) {
	case 0:
		return Resolution{Status: StatusNotFound}, nil
	case 1:
		return s.openCandidate(ctx, strings.TrimSpace(exact[0].ProviderID), exact[0].Title)
	}
	return Resolution{Status: StatusAmbiguous}, nil
}

// takeSearch counts the attempt before the request goes out, so a page opening
// links in a loop cannot reach the metadata service past the limit.
func (s *Service) takeSearch() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	if s.searches >= maxTitleSearches || (s.searches > 0 && now.Sub(s.lastSearch) < minTitleSearchGap) {
		return ErrTooManyTitleSearches
	}
	s.searches++
	s.lastSearch = now
	return nil
}

// mainCandidates keeps the distinct games with exactly this title. A bundle is
// how IGDB stores some editions, so it stays; DLC, updates and other add-ons go.
// An edition listed next to its parent under the same title is the same game.
func mainCandidates(candidates []metadata.Candidate, normalized string) []metadata.Candidate {
	byID := map[string]metadata.Candidate{}
	for _, candidate := range candidates {
		if titles.Normalize(candidate.Title) != normalized || !isMainGame(candidate.GameType) {
			continue
		}
		byID[strings.TrimSpace(candidate.ProviderID)] = candidate
	}
	exact := make([]metadata.Candidate, 0, len(byID))
	for _, candidate := range byID {
		if _, listed := byID[strings.TrimSpace(candidate.ParentID)]; !listed {
			exact = append(exact, candidate)
		}
	}
	return exact
}

func isMainGame(gameType string) bool {
	return gameType == gameTypeBundle || titles.IsGameType(gameType)
}

func (s *Service) openCandidate(ctx context.Context, providerID, title string) (Resolution, error) {
	if steamID, steam := strings.CutPrefix(providerID, steamProviderPrefix); steam {
		if err := validateID(steamID); err != nil {
			return Resolution{}, fmt.Errorf("%w: %w", ErrBadProviderID, err)
		}
		return Resolution{Status: StatusNotInCatalog}, nil
	}
	if err := validateID(providerID); err != nil {
		return Resolution{}, fmt.Errorf("%w: %w", ErrBadProviderID, err)
	}
	return s.open(ctx, providerID, title)
}

// titleLookupError never wraps a transport error: url.Error quotes the request
// URL, and that URL holds the title from the link.
func titleLookupError(err error) error {
	switch {
	case uierr.Code(err) != "":
		return err
	case errors.Is(err, typhonapi.ErrOutdated):
		return uierr.New(codeOutdated, typhonapi.ErrOutdated.Error())
	case errors.Is(err, typhonapi.ErrBadRequest):
		return uierr.New(codeBadRequest, typhonapi.ErrBadRequest.Error())
	case errors.Is(err, typhonapi.ErrUpstream):
		return uierr.New(codeUnavailable, typhonapi.ErrUpstream.Error())
	}
	slog.Warn("look up title from link", "component", "deeplink", "operation", "resolve_title", "errorType", fmt.Sprintf("%T", err))
	return errTitleLookup
}

func lookupError(err error) error {
	switch {
	case uierr.Code(err) != "":
		return err
	case errors.Is(err, typhonapi.ErrOutdated):
		return uierr.Wrap(codeOutdated, err)
	case errors.Is(err, typhonapi.ErrBadRequest):
		return uierr.Wrap(codeBadRequest, err)
	case errors.Is(err, typhonapi.ErrUpstream):
		return uierr.Wrap(codeUnavailable, err)
	}
	return fmt.Errorf("look up game from link: %w", err)
}

// open holds the mutex across the check and the write: two links racing for the
// last slot must not both create a record.
func (s *Service) open(ctx context.Context, igdbID, title string) (Resolution, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return Resolution{}, err
	}
	if game, ok := s.catalog.GameByIGDB(igdbID); ok {
		return found(game.ID), nil
	}
	if s.created >= maxNewGames {
		return Resolution{}, ErrTooManyNewGames
	}
	game, err := s.catalog.EnsureByIGDB(igdbID, title)
	if err != nil {
		return Resolution{}, err
	}
	s.created++
	return found(game.ID), nil
}

func (s *Service) localGame(target Target) (string, bool) {
	if target.Provider == ProviderIGDB {
		game, ok := s.catalog.GameByIGDB(target.ID)
		return game.ID, ok
	}
	match := s.catalog.Resolve(catalog.Query{ExternalIDs: catalog.ExternalIDs{Steam: target.ID}})
	return match.GameID, match.Status == catalog.StatusMatched && match.GameID != ""
}

func found(id string) Resolution {
	return Resolution{Status: StatusFound, GameID: id}
}

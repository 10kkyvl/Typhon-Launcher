package deeplink

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

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
)

var (
	ErrTooManyNewGames = uierr.New("link.too_many", "по ссылкам добавлено слишком много новых игр, перезапустите лаунчер")
	ErrBadProviderID   = uierr.New("link.bad_response", "сервер метаданных вернул некорректный идентификатор игры")

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

	mu      sync.Mutex
	created int
}

func NewService(games Catalog, lookup Metadata, language func() string) (*Service, error) {
	if games == nil || lookup == nil || language == nil {
		return nil, errNoDependency
	}
	return &Service{catalog: games, metadata: lookup, language: language}, nil
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

// ResolveTitle finds the one game a search link names: on this machine first,
// then by asking the metadata service for exactly this title. Only the page calls
// it, and the page waits for the accepted terms and a session before taking a
// link. A catalog record appears only for a single exact match, under the same
// limit as a game link. Candidates are read instead of Match.Status because
// catalog.Resolve never reports a title as matched next to a remote catalog.
func (s *Service) ResolveTitle(ctx context.Context, query string) (Resolution, error) {
	if err := ctx.Err(); err != nil {
		return Resolution{}, err
	}
	title, err := validateQuery(query)
	if err != nil {
		return Resolution{}, err
	}
	local := s.localTitle(title)
	if err := ctx.Err(); err != nil {
		return Resolution{}, err
	}
	if local.Status == StatusFound {
		return local, nil
	}
	normalized := titles.Normalize(title)
	if normalized == "" {
		return local, nil
	}
	candidates, err := s.metadata.SearchTitle(metadata.WithLanguage(ctx, s.language()), title)
	if errors.Is(err, metadata.ErrNoMatch) {
		return local, nil
	}
	if err != nil {
		if ctx.Err() != nil {
			return Resolution{}, ctx.Err()
		}
		return Resolution{}, titleLookupError(err)
	}
	var exact *metadata.Candidate
	for i := range candidates {
		if titles.Normalize(candidates[i].Title) != normalized {
			continue
		}
		if exact != nil && exact.ProviderID != candidates[i].ProviderID {
			return Resolution{Status: StatusAmbiguous}, nil
		}
		exact = &candidates[i]
	}
	if exact == nil {
		return local, nil
	}
	return s.openCandidate(ctx, strings.TrimSpace(exact.ProviderID), exact.Title)
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

func (s *Service) localTitle(title string) Resolution {
	match := s.catalog.Resolve(catalog.Query{Title: title})
	if match.Method == catalog.MethodOverride && match.GameID != "" {
		return found(match.GameID)
	}
	exact := ""
	for _, candidate := range match.Candidates {
		if candidate.GameID == "" || (candidate.Method != catalog.MethodExactTitle && candidate.Method != catalog.MethodAlias) {
			continue
		}
		if exact != "" {
			return Resolution{Status: StatusAmbiguous}
		}
		exact = candidate.GameID
	}
	if exact == "" {
		return Resolution{Status: StatusNotFound}
	}
	return found(exact)
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
	}
	return uierr.New(codeUnavailable, typhonapi.ErrUpstream.Error())
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

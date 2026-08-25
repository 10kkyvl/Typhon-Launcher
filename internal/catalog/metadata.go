package catalog

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"typhon/internal/titles"
)

var (
	ErrNoGame      = errNotFound
	errNoProvider  = errors.New("не указан идентификатор провайдера")
	errNoTimestamp = errors.New("не указано время обновления метаданных")
)

type MetadataPatch struct {
	IGDBID       string
	Title        string
	Summary      string
	ReleaseDate  *time.Time
	Developer    string
	Publisher    string
	Genres       []string
	Themes       []string
	Platforms    []string
	CoverAssetID string
	HeroAssetID  string
	UpdatedAt    time.Time
	Partial      bool
}

//wails:ignore
func (s *Service) ApplyMetadata(gameID string, patch MetadataPatch) (Game, error) {
	gameID = strings.TrimSpace(gameID)
	if gameID == "" {
		return Game{}, ErrNoGame
	}
	if strings.TrimSpace(patch.IGDBID) == "" {
		return Game{}, errNoProvider
	}
	if patch.UpdatedAt.IsZero() {
		return Game{}, errNoTimestamp
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	pos, err := s.positionLocked(gameID)
	if err != nil {
		return Game{}, err
	}
	previous := s.games[pos]
	s.games[pos] = applyPatch(previous, patch)
	s.rebuildLocked()

	if err := s.persistGamesLocked(); err != nil {
		s.games[pos] = previous
		s.rebuildLocked()
		return Game{}, fmt.Errorf("save catalog: %w", err)
	}
	return s.games[pos], nil
}

func (s *Service) positionLocked(gameID string) (int, error) {
	pos, ok := s.idx.byID[gameID]
	if !ok || pos >= len(s.games) || s.games[pos].ID != gameID {
		for i := range s.games {
			if s.games[i].ID == gameID {
				return i, nil
			}
		}
		return 0, ErrNoGame
	}
	return pos, nil
}

func applyPatch(game Game, patch MetadataPatch) Game {
	game.ExternalIDs.IGDB = strings.TrimSpace(patch.IGDBID)
	game = applyTitle(game, strings.TrimSpace(patch.Title))
	game.Summary = patch.Summary
	game.Developer = patch.Developer
	game.Publisher = patch.Publisher
	game.Genres = copyStrings(patch.Genres)
	game.Themes = copyStrings(patch.Themes)
	game.Platforms = copyStrings(patch.Platforms)
	if patch.ReleaseDate != nil {
		released := *patch.ReleaseDate
		game.ReleaseDate = &released
		year := released.Year()
		game.ReleaseYear = &year
	}
	game.CoverAssetID = patch.CoverAssetID
	game.HeroAssetID = patch.HeroAssetID
	game.MetadataPartial = patch.Partial
	updated := patch.UpdatedAt
	game.MetadataUpdatedAt = &updated
	return game
}

func applyTitle(game Game, title string) Game {
	if title == "" {
		return game
	}
	previous := titles.Normalize(game.Title)
	if game.Provisional {
		game.Title = title
		game.SortTitle = sortTitle(title)
		game.Provisional = false
		return rememberAlias(game, previous)
	}
	return rememberAlias(game, titles.Normalize(title))
}

func rememberAlias(game Game, alias string) Game {
	if alias == "" || !learnable(alias, game) {
		return game
	}
	aliases := make([]string, 0, len(game.Aliases)+1)
	aliases = append(aliases, game.Aliases...)
	game.Aliases = append(aliases, alias)
	return game
}

func copyStrings(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, 0, len(in))
	for _, v := range in {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		out = append(out, v)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

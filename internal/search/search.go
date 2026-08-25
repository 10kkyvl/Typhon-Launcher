package search

import (
	"sort"
	"strings"

	"typhon/internal/catalog"
	"typhon/internal/library"
	"typhon/internal/sources"
	"typhon/internal/titles"
)

const (
	minQueryLen        = 1
	minReleaseQueryLen = 2
	maxGames           = 10
	maxReleases        = 5
	catalogScan        = 32
)

const (
	scoreExactTitle      = 100
	scoreExactAlias      = 80
	scorePrefixTitle     = 60
	scorePrefixAlias     = 50
	scoreSubstringTitle  = 30
	scoreSubstringAlias  = 25
	scoreNormalizedTitle = 20
	scoreNormalizedAlias = 15
	scoreReleaseOnly     = 10
	scoreCatalogFuzzy    = 5
)

type GameHit struct {
	ID              string  `json:"id"`
	Title           string  `json:"title"`
	Cover           string  `json:"cover,omitempty"`
	Year            int     `json:"year,omitempty"`
	Installed       bool    `json:"installed"`
	CanonicalGameID string  `json:"canonicalGameId,omitempty"`
	Version         string  `json:"version,omitempty"`
	LatestVersion   string  `json:"latestVersion,omitempty"`
	Releases        int     `json:"releases"`
	Sources         int     `json:"sources"`
	Score           float64 `json:"score"`
}

type ReleaseHit struct {
	ID         string `json:"id"`
	SourceID   string `json:"sourceId"`
	SourceName string `json:"sourceName"`
	Title      string `json:"title"`
	Version    string `json:"version,omitempty"`
	Size       int64  `json:"size"`
}

type Result struct {
	Query        string       `json:"query"`
	Games        []GameHit    `json:"games"`
	Releases     []ReleaseHit `json:"releases"`
	MoreGames    int          `json:"moreGames"`
	MoreReleases int          `json:"moreReleases"`
}

type installedGames interface {
	GetInstalledGames() []library.Game
}

type gameCatalog interface {
	SearchGames(query string, limit int) []catalog.Game
	GetGame(id string) (catalog.Game, error)
}

type releaseIndex interface {
	SearchReleaseMatches(query string, gameIDs []string, unmatchedLimit int) sources.ReleaseMatches
}

type Service struct {
	library installedGames
	catalog gameCatalog
	sources releaseIndex
}

func NewService(libraryService installedGames, catalogService gameCatalog, sourcesService releaseIndex) *Service {
	return &Service{library: libraryService, catalog: catalogService, sources: sourcesService}
}

type entry struct {
	hit GameHit
}

type query struct {
	raw        string
	lower      string
	normalized string
}

func (s *Service) Search(raw string) Result {
	trimmed := strings.Join(strings.Fields(raw), " ")
	result := Result{Query: trimmed, Games: []GameHit{}, Releases: []ReleaseHit{}}
	if len([]rune(trimmed)) < minQueryLen {
		return result
	}
	q := query{raw: trimmed, lower: strings.ToLower(trimmed), normalized: titles.Normalize(trimmed)}

	installed := s.installed()
	entries := map[string]*entry{}
	s.collectCatalog(entries, q)
	collectInstalled(entries, installed, q)
	unmatched, moreUnmatched := s.collectReleases(entries, installed, q)

	games := make([]GameHit, 0, len(entries))
	for _, e := range entries {
		games = append(games, e.hit)
	}
	sortGames(games)
	if len(games) > maxGames {
		result.MoreGames = len(games) - maxGames
		games = games[:maxGames]
	}
	result.Games = games
	result.Releases = unmatched
	result.MoreReleases = moreUnmatched
	return result
}

func (s *Service) installed() []library.Game {
	if s.library == nil {
		return nil
	}
	return s.library.GetInstalledGames()
}

func (s *Service) collectCatalog(entries map[string]*entry, q query) {
	if s.catalog == nil {
		return
	}
	for _, game := range s.catalog.SearchGames(q.raw, catalogScan) {
		e := ensure(entries, game.ID)
		applyCatalog(e, game)
		e.hit.Score = maxScore(e.hit.Score, q.score(game.Title, game.Aliases), scoreCatalogFuzzy)
	}
}

func collectInstalled(entries map[string]*entry, installed []library.Game, q query) {
	for _, game := range installed {
		hit := q.score(game.Title, nil)
		key := game.CanonicalGameID
		if key == "" {
			if hit == 0 {
				continue
			}
			key = game.ID
		}
		e, known := entries[key]
		if !known {
			if hit == 0 {
				continue
			}
			e = ensure(entries, key)
		}
		applyInstalled(e, game)
		e.hit.Score = maxScore(e.hit.Score, hit)
	}
}

func (s *Service) collectReleases(entries map[string]*entry, installed []library.Game, q query) ([]ReleaseHit, int) {
	if s.sources == nil || len([]rune(q.raw)) < minReleaseQueryLen {
		return []ReleaseHit{}, 0
	}
	known := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.hit.CanonicalGameID != "" {
			known = append(known, e.hit.CanonicalGameID)
		}
	}
	matches := s.sources.SearchReleaseMatches(q.raw, known, maxReleases)

	for gameID, info := range matches.Games {
		e, seen := entries[gameID]
		if !seen {
			e = ensure(entries, gameID)
			e.hit.ID = gameID
			e.hit.CanonicalGameID = gameID
			e.hit.Title = info.Title
			s.fillFromCatalog(e, gameID, q)
			for _, game := range installed {
				if game.CanonicalGameID == gameID {
					applyInstalled(e, game)
				}
			}
			e.hit.Score = maxScore(e.hit.Score, scoreReleaseOnly)
		}
		e.hit.Releases = info.Releases
		e.hit.Sources = info.Sources
		e.hit.LatestVersion = info.LatestVersion
	}

	hits := make([]ReleaseHit, 0, len(matches.Unmatched))
	for _, view := range matches.Unmatched {
		hits = append(hits, ReleaseHit{
			ID:         view.Release.ID,
			SourceID:   view.Release.SourceID,
			SourceName: view.SourceName,
			Title:      view.Release.RawTitle,
			Version:    view.Release.Version,
			Size:       view.Release.Size,
		})
	}
	return hits, matches.MoreUnmatched
}

func (s *Service) fillFromCatalog(e *entry, gameID string, q query) {
	if s.catalog == nil {
		return
	}
	game, err := s.catalog.GetGame(gameID)
	if err != nil {
		return
	}
	applyCatalog(e, game)
	e.hit.Score = maxScore(e.hit.Score, q.score(game.Title, game.Aliases))
}

func applyCatalog(e *entry, game catalog.Game) {
	e.hit.CanonicalGameID = game.ID
	if e.hit.ID == "" {
		e.hit.ID = game.ID
	}
	if game.Title != "" {
		e.hit.Title = game.Title
	}
	if game.ReleaseYear != nil {
		e.hit.Year = *game.ReleaseYear
	}
}

func applyInstalled(e *entry, game library.Game) {
	if e.hit.Installed && e.hit.ID <= game.ID {
		return
	}
	e.hit.ID = game.ID
	e.hit.Installed = true
	e.hit.Version = game.Version
	if game.CanonicalGameID != "" {
		e.hit.CanonicalGameID = game.CanonicalGameID
	}
	if game.Cover != "" {
		e.hit.Cover = game.Cover
	}
	if e.hit.Title == "" {
		e.hit.Title = game.Title
	}
}

func ensure(entries map[string]*entry, key string) *entry {
	if e, ok := entries[key]; ok {
		return e
	}
	e := &entry{}
	entries[key] = e
	return e
}

func (q query) score(title string, aliases []string) float64 {
	best := q.match(title, scoreExactTitle, scorePrefixTitle, scoreSubstringTitle, scoreNormalizedTitle)
	for _, alias := range aliases {
		if value := q.match(alias, scoreExactAlias, scorePrefixAlias, scoreSubstringAlias, scoreNormalizedAlias); value > best {
			best = value
		}
	}
	return best
}

func (q query) match(candidate string, exact, prefix, substring, normalized float64) float64 {
	if candidate == "" {
		return 0
	}
	lowered := strings.ToLower(candidate)
	switch {
	case lowered == q.lower:
		return exact
	case strings.HasPrefix(lowered, q.lower):
		return prefix
	case strings.Contains(lowered, q.lower):
		return substring
	}
	if q.normalized != "" && strings.Contains(titles.Normalize(candidate), q.normalized) {
		return normalized
	}
	return 0
}

func maxScore(values ...float64) float64 {
	best := values[0]
	for _, value := range values[1:] {
		if value > best {
			best = value
		}
	}
	return best
}

func sortGames(games []GameHit) {
	sort.Slice(games, func(a, b int) bool {
		left, right := games[a], games[b]
		if left.Score != right.Score {
			return left.Score > right.Score
		}
		if left.Installed != right.Installed {
			return left.Installed
		}
		if left.Releases != right.Releases {
			return left.Releases > right.Releases
		}
		if left.Title != right.Title {
			return left.Title < right.Title
		}
		return left.ID < right.ID
	})
}

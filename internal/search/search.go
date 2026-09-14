package search

import (
	"errors"
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
	BrowseGames(catalog.GameQuery) (catalog.GamePage, error)
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

func (s *Service) Search(raw string) (Result, error) {
	trimmed := strings.Join(strings.Fields(raw), " ")
	result := Result{Query: trimmed, Games: []GameHit{}, Releases: []ReleaseHit{}}
	if len([]rune(trimmed)) < minQueryLen {
		return result, nil
	}
	q := query{raw: trimmed, lower: strings.ToLower(trimmed), normalized: titles.Normalize(trimmed)}

	installed := s.installed()
	entries := map[string]*entry{}
	total, err := s.collectCatalog(entries, q)
	if err != nil {
		return result, err
	}
	collectInstalled(entries, installed)
	s.collectReleases(entries, q)

	games := make([]GameHit, 0, len(entries))
	seen := map[*entry]bool{}
	for _, e := range entries {
		if !seen[e] {
			games = append(games, e.hit)
			seen[e] = true
		}
	}
	sortGames(games)
	if len(games) > maxGames {
		result.MoreGames = max(0, total-maxGames)
		games = games[:maxGames]
	}
	result.Games = games
	return result, nil
}

func (s *Service) installed() []library.Game {
	if s.library == nil {
		return nil
	}
	return s.library.GetInstalledGames()
}

func (s *Service) collectCatalog(entries map[string]*entry, q query) (int, error) {
	if s.catalog == nil {
		return 0, errors.New("catalog backend unavailable")
	}
	page, err := s.catalog.BrowseGames(catalog.GameQuery{Search: q.raw, Kind: "all", Page: 1, PageSize: catalogScan, Sort: "title"})
	if err != nil {
		return 0, err
	}
	for _, game := range page.Items {
		e := ensure(entries, game.ID)
		applyCatalog(e, game)
		e.hit.Score = maxScore(e.hit.Score, q.score(game.Title, game.Aliases), scoreCatalogFuzzy)
		for _, alias := range game.AliasIDs {
			entries[alias] = e
		}
	}
	return page.Total, nil
}

func collectInstalled(entries map[string]*entry, installed []library.Game) {
	for _, game := range installed {
		if e := entries[game.CanonicalGameID]; e != nil {
			applyInstalled(e, game)
		}
	}
}

func (s *Service) collectReleases(entries map[string]*entry, q query) {
	if s.sources == nil || len([]rune(q.raw)) < minReleaseQueryLen {
		return
	}
	known := make([]string, 0, len(entries))
	for id := range entries {
		known = append(known, id)
	}
	// Only server-selected identities receive source annotations. Source titles
	// and unmatched releases cannot introduce search results.
	matches := s.sources.SearchReleaseMatches(q.raw, known, 0)
	for gameID, info := range matches.Games {
		if e := entries[gameID]; e != nil {
			e.hit.Releases += info.Releases
			e.hit.Sources += info.Sources
			e.hit.LatestVersion = info.LatestVersion
		}
	}
}

func applyCatalog(e *entry, game catalog.Game) {
	e.hit.CanonicalGameID = game.ID
	e.hit.Cover = game.CoverURL
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

package catalog

import (
	"sort"
	"strings"

	"typhon/internal/titles"
)

const (
	defaultPageSize = 60
	maxPageSize     = 200
)

var genreGroups = []struct {
	label   string
	sources []string
}{
	{"Экшен", []string{"Action", "Fighting", "Hack and slash/Beat 'em up"}},
	{"Ролевые", []string{"Role-playing (RPG)", "RPG"}},
	{"Шутеры", []string{"Shooter"}},
	{"Приключения", []string{"Adventure", "Point-and-click", "Platform", "Platformer"}},
	{"Стратегии", []string{"Strategy", "Real Time Strategy (RTS)", "Turn-based strategy (TBS)", "Tactical"}},
	{"Инди", []string{"Indie"}},
}

type GameQuery struct {
	Search   string `json:"search"`
	Genre    string `json:"genre"`
	Sort     string `json:"sort"`
	Page     int    `json:"page"`
	PageSize int    `json:"pageSize"`
}

type GamePage struct {
	Items    []Game `json:"items"`
	Total    int    `json:"total"`
	Page     int    `json:"page"`
	PageSize int    `json:"pageSize"`
}

type GenreFacet struct {
	Label string `json:"label"`
	Count int    `json:"count"`
}

func (s *Service) QueryGames(q GameQuery) GamePage {
	if q.PageSize <= 0 {
		q.PageSize = defaultPageSize
	}
	if q.PageSize > maxPageSize {
		q.PageSize = maxPageSize
	}
	if q.Page <= 0 {
		q.Page = 1
	}
	search := strings.ToLower(strings.TrimSpace(q.Search))
	normalized := titles.Normalize(search)
	genre := strings.TrimSpace(q.Genre)

	s.mu.Lock()
	defer s.mu.Unlock()

	filtered := make([]Game, 0, len(s.idx.entries))
	for i := range s.idx.entries {
		e := &s.idx.entries[i]
		if search != "" && !entryMatches(e, search, normalized) {
			continue
		}
		if genre != "" && !genreMatches(e.game.Genres, genre) {
			continue
		}
		filtered = append(filtered, e.game)
	}
	sortGames(filtered, q.Sort)

	total := len(filtered)
	start := min((q.Page-1)*q.PageSize, total)
	end := min(start+q.PageSize, total)
	return GamePage{Items: filtered[start:end], Total: total, Page: q.Page, PageSize: q.PageSize}
}

func (s *Service) GenreFacets() []GenreFacet {
	s.mu.Lock()
	defer s.mu.Unlock()

	counts := make([]int, len(genreGroups))
	for i := range s.idx.entries {
		genres := s.idx.entries[i].game.Genres
		for gi, group := range genreGroups {
			if genresMatchAny(genres, group.sources) {
				counts[gi]++
			}
		}
	}

	out := make([]GenreFacet, len(genreGroups))
	for i, group := range genreGroups {
		out[i] = GenreFacet{Label: group.label, Count: counts[i]}
	}
	return out
}

func genreMatches(genres []string, label string) bool {
	for _, group := range genreGroups {
		if strings.EqualFold(group.label, label) {
			return genresMatchAny(genres, group.sources)
		}
	}
	return false
}

func genresMatchAny(genres, sources []string) bool {
	for _, g := range genres {
		for _, src := range sources {
			if strings.EqualFold(g, src) {
				return true
			}
		}
	}
	return false
}

func (s *Service) GetGames(ids []string) []Game {
	if len(ids) == 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Game, 0, len(ids))
	seen := make(map[string]bool, len(ids))
	for _, id := range ids {
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		game, ok := s.idx.game(id)
		if !ok {
			continue
		}
		out = append(out, game)
	}
	return out
}

func entryMatches(e *entry, search, normalized string) bool {
	if strings.Contains(strings.ToLower(e.game.Title), search) {
		return true
	}
	if normalized != "" && strings.Contains(e.normalized, normalized) {
		return true
	}
	for _, alias := range e.aliases {
		if normalized != "" && strings.Contains(alias, normalized) {
			return true
		}
	}
	for _, alias := range e.game.Aliases {
		if strings.Contains(strings.ToLower(alias), search) {
			return true
		}
	}
	return false
}

func sortGames(list []Game, mode string) {
	switch mode {
	case "year":
		sort.Slice(list, func(a, b int) bool {
			left, right := list[a].ReleaseYear, list[b].ReleaseYear
			switch {
			case left == nil && right == nil:
			case left == nil:
				return false
			case right == nil:
				return true
			case *left != *right:
				return *left > *right
			}
			return lessByTitle(list[a], list[b])
		})
	case "added":
		sort.Slice(list, func(a, b int) bool {
			if !list[a].CreatedAt.Equal(list[b].CreatedAt) {
				return list[a].CreatedAt.After(list[b].CreatedAt)
			}
			return lessByTitle(list[a], list[b])
		})
	default:
		sort.Slice(list, func(a, b int) bool { return lessByTitle(list[a], list[b]) })
	}
}

func lessByTitle(a, b Game) bool {
	if a.SortTitle != b.SortTitle {
		return a.SortTitle < b.SortTitle
	}
	return a.ID < b.ID
}

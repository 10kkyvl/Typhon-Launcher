package catalog

import (
	"sort"
	"strings"
	"time"

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
	Platform string `json:"platform"`
	Kind     string `json:"kind"`
	Revision int64  `json:"revision"`
	Search   string `json:"search"`
	Genre    string `json:"genre"`
	Sort     string `json:"sort"`
	Compat   string `json:"compat"`
	Page     int    `json:"page"`
	PageSize int    `json:"pageSize"`
}

// CompatOnlyWorking — значение GameQuery.Compat, оставляющее только игры,
// которые у людей запускаются. Фильтр применяется до нарезки на страницы:
// отсеивать на фронте значит отдавать страницы разной длины.
const CompatOnlyWorking = "works"

type IndexStatus struct {
	Provider  string     `json:"provider"`
	Complete  bool       `json:"complete"`
	UpdatedAt *time.Time `json:"updatedAt,omitempty"`
	Records   int64      `json:"records"`
}
type GamePage struct {
	Facets    []GenreFacet  `json:"facets"`
	Platforms []GenreFacet  `json:"platforms"`
	Offline   bool          `json:"offline"`
	CachedAt  time.Time     `json:"cachedAt"`
	Revision  int64         `json:"revision"`
	Providers []IndexStatus `json:"providers"`
	Items     []Game        `json:"items"`
	// Compat отдаётся отдельной картой, а не полем Game: общая статистика
	// приходит с сервера и меняется сама по себе, а Game лежит на диске.
	Compat   map[string]CompatInfo `json:"compat,omitempty"`
	Total    int                   `json:"total"`
	Page     int                   `json:"page"`
	PageSize int                   `json:"pageSize"`
}

// CompatInfo — сколько машин из скольких запустили эту игру. Доля считается на
// фронте: показывать её и «мало данных» — решение интерфейса.
type CompatInfo struct {
	Works int `json:"works"`
	Total int `json:"total"`
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
		g := s.idx.games[i]
		if search != "" && !entryMatches(e, g, search, normalized) {
			continue
		}
		if genre != "" && !genreMatches(g.Genres, genre) {
			continue
		}
		if q.Compat == CompatOnlyWorking && !s.compatWorksLocked(g.ID) {
			continue
		}
		filtered = append(filtered, g)
	}
	sortGames(filtered, q.Sort)

	total := len(filtered)
	start := min((q.Page-1)*q.PageSize, total)
	end := min(start+q.PageSize, total)
	items := filtered[start:end]
	return GamePage{
		Items:    items,
		Compat:   s.compatForLocked(items),
		Total:    total,
		Page:     q.Page,
		PageSize: q.PageSize,
	}
}

// SetCompatLookup связывает каталог с общей статистикой. Каталог о ней ничего
// не знает и без неё работает так же, только молчит про чужие машины.
//
// Колбэк спрашивают по идентификатору IGDB, а не по каноническому: перевод
// одного в другой — работа каталога, и делать её обязан он сам. Колбэк,
// которому пришлось бы дёргать каталог обратно, звался бы под его же
// мьютексом и вешал бы запрос намертво.
//
//wails:ignore
func (s *Service) SetCompatLookup(fn func(igdbID string) (works, total int, ok bool)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.compat = fn
}

// compatWorksLocked отвечает на вопрос фильтра. Игра без наблюдений в «поедет»
// не попадает: молчание — не то же самое, что подтверждённая работоспособность.
func (s *Service) compatWorksLocked(gameID string) bool {
	works, total, ok := s.compatLocked(gameID)
	return ok && total > 0 && works*2 > total
}

func (s *Service) compatLocked(gameID string) (works, total int, ok bool) {
	if s.compat == nil {
		return 0, 0, false
	}
	igdbID := s.igdbIDLocked(gameID)
	if igdbID == "" {
		return 0, 0, false
	}
	return s.compat(igdbID)
}

func (s *Service) compatForLocked(items []Game) map[string]CompatInfo {
	if s.compat == nil {
		return nil
	}
	out := make(map[string]CompatInfo, len(items))
	for i := range items {
		if works, total, ok := s.compatLocked(items[i].ID); ok {
			out[items[i].ID] = CompatInfo{Works: works, Total: total}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func (s *Service) GenreFacets() []GenreFacet {
	s.mu.Lock()
	defer s.mu.Unlock()

	counts := make([]int, len(genreGroups))
	for i := range s.idx.entries {
		genres := s.idx.games[i].Genres
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

func entryMatches(e *entry, g Game, search, normalized string) bool {
	if strings.Contains(strings.ToLower(g.Title), search) {
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
	for _, alias := range g.Aliases {
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

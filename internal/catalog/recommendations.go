package catalog

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"time"

	"typhon/internal/storage"
	"typhon/internal/uierr"
)

const (
	recommendationVersion      = 1
	personalizationMinGames    = 3
	personalizationMinPlaytime = int64(3 * 60 * 60)
	minMeaningfulPlaytime      = int64(30 * 60)
	maxDiscoveryItems          = 5
)

var (
	errInvalidRecommendationSort = uierr.New("catalog.invalid_recommendation_sort", "неизвестная сортировка каталога")
	errEmptyRecommendationID     = uierr.New("catalog.empty_recommendation_id", "не указан идентификатор игры")
)

// RecommendationLibraryItem is the small, stable boundary between catalog
// ranking and the launcher library. The catalog deliberately does not import
// library: main wires this snapshot callback after both services start.
type RecommendationLibraryItem struct {
	LibraryID       string     `json:"libraryId,omitempty"`
	CanonicalGameID string     `json:"canonicalGameId,omitempty"`
	Favorite        bool       `json:"favorite,omitempty"`
	PlaytimeSeconds int64      `json:"playtimeSeconds,omitempty"`
	Sessions        int        `json:"sessions,omitempty"`
	LastPlayed      *time.Time `json:"lastPlayed,omitempty"`
	Installed       bool       `json:"installed,omitempty"`
	Hidden          bool       `json:"hidden,omitempty"`
	ContinuePlaying bool       `json:"continuePlaying,omitempty"`
}

type RecommendationLibrarySource func() []RecommendationLibraryItem

// RecommendationPreferences are user choices and explicit negative signals.
// They are local durable state; account sync can carry them later without
// making recommendations depend on a network round trip.
type RecommendationPreferences struct {
	DefaultSort       string   `json:"defaultSort,omitempty"`
	Genre             string   `json:"genre,omitempty"`
	Platform          string   `json:"platform,omitempty"`
	Kind              string   `json:"kind,omitempty"`
	Compat            string   `json:"compat,omitempty"`
	CompatOnly        bool     `json:"compatOnly,omitempty"`
	HideLibrary       bool     `json:"hideLibrary,omitempty"`
	HideNotInterested bool     `json:"hideNotInterested"`
	NotInterested     []string `json:"notInterested,omitempty"`
}

type RecommendationProfile struct {
	DefaultSort      string                `json:"defaultSort"`
	Confidence       float64               `json:"confidence"`
	EvidenceGames    int                   `json:"evidenceGames"`
	EvidencePlaytime int64                 `json:"evidencePlaytimeSeconds"`
	Genres           []RecommendationFacet `json:"genres,omitempty"`
	Themes           []RecommendationFacet `json:"themes,omitempty"`
}

type RecommendationFacet struct {
	Value string  `json:"value"`
	Score float64 `json:"score"`
}

type RecommendationItem struct {
	Game        Game   `json:"game"`
	LibraryID   string `json:"libraryId,omitempty"`
	Reason      string `json:"reason"`
	ReasonTitle string `json:"reasonTitle,omitempty"`
	ReasonGenre string `json:"reasonGenre,omitempty"`
}

type DiscoveryQuery struct {
	GameQuery
	RefreshExcludeIDs []string `json:"refreshExcludeIds,omitempty"`
	Limit             int      `json:"limit,omitempty"`
}

type DiscoveryResult struct {
	Items    []RecommendationItem  `json:"items"`
	Fallback bool                  `json:"fallback"`
	Profile  RecommendationProfile `json:"profile"`
}

type LibraryRecommendationQuery struct {
	Limit             int      `json:"limit,omitempty"`
	ExcludeLibraryIDs []string `json:"excludeLibraryIds,omitempty"`
	ExcludeGameIDs    []string `json:"excludeGameIds,omitempty"`
}

type recommendationState struct {
	Preferences RecommendationPreferences `json:"preferences"`
}

type recommendationEvidence struct {
	Genres   map[string]float64
	Themes   map[string]float64
	Games    int
	Playtime int64
}

func (s *Service) enrichRecommendationQuery(q GameQuery) GameQuery {
	games, p, source := s.recommendationSnapshot()
	if q.Sort == "for-you" && strings.TrimSpace(q.Profile) == "" {
		profile := profileFromEvidence(buildEvidence(games, func() []RecommendationLibraryItem {
			if source == nil {
				return nil
			}
			return source()
		}()), p)
		weights := make(map[string]float64, len(profile.Genres))
		for _, facet := range profile.Genres {
			weights[facet.Value] = facet.Score
		}
		themes := make(map[string]float64, len(profile.Themes))
		for _, facet := range profile.Themes {
			themes[facet.Value] = facet.Score
		}
		payload, _ := json.Marshal(struct {
			Genres   map[string]float64 `json:"genres,omitempty"`
			Themes   map[string]float64 `json:"themes,omitempty"`
			Strength float64            `json:"strength,omitempty"`
		}{Genres: weights, Themes: themes, Strength: profile.Confidence})
		q.Profile = string(payload)
	}
	if q.HideNotInterested {
		ids := append([]string(nil), p.NotInterested...)
		ids = append(ids, q.ExcludeIDs...)
		q.ExcludeNotInterested = strings.Join(mergeIDs(ids), ",")
	}
	if q.HideLibrary && source != nil {
		items := source()
		ids := make([]string, 0, len(items))
		for _, item := range items {
			if item.Hidden {
				continue
			}
			if item.CanonicalGameID != "" {
				ids = append(ids, item.CanonicalGameID)
			}
		}
		q.ExcludeLibrary = strings.Join(mergeIDs(ids), ",")
	}
	return q
}

func (s *Service) loadRecommendations() error {
	path := s.recommendationPath
	if path == "" {
		return errors.New("recommendation path unavailable")
	}
	var state recommendationState
	err := storage.Load(path, recommendationVersion, nil, &state)
	if err != nil {
		// A missing preferences file is the expected first-run state.
		if errors.Is(err, fs.ErrNotExist) {
			s.preferences = defaultRecommendationPreferences()
			return nil
		}
		return fmt.Errorf("load recommendation preferences: %w", err)
	}
	s.preferences = sanitizeRecommendationPreferences(state.Preferences)
	return nil
}

func defaultRecommendationPreferences() RecommendationPreferences {
	return RecommendationPreferences{DefaultSort: "", HideNotInterested: true}
}

func sanitizeRecommendationPreferences(p RecommendationPreferences) RecommendationPreferences {
	if !validRecommendationSort(p.DefaultSort) {
		p.DefaultSort = ""
	}
	p.Genre = strings.TrimSpace(p.Genre)
	p.Platform = strings.TrimSpace(p.Platform)
	p.Kind = strings.TrimSpace(p.Kind)
	p.Compat = strings.TrimSpace(p.Compat)
	seen := map[string]bool{}
	ids := make([]string, 0, len(p.NotInterested))
	for _, id := range p.NotInterested {
		id = strings.TrimSpace(id)
		if id != "" && !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	p.NotInterested = ids
	return p
}

func validRecommendationSort(sortName string) bool {
	switch sortName {
	case "", "auto", "for-you", "popular", "rating", "year", "newest", "title", "added":
		return true
	default:
		return false
	}
}

// SetRecommendationLibrarySource wires a read-only library snapshot. The
// callback is called outside the catalog mutex, so it may safely query the
// library service.
//
//wails:ignore
func (s *Service) SetRecommendationLibrarySource(source RecommendationLibrarySource) {
	s.mu.Lock()
	s.recommendationLibrary = source
	s.mu.Unlock()
}

// GetRecommendationPreferences returns a defensive copy.
func (s *Service) GetRecommendationPreferences() RecommendationPreferences {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p := s.preferences
	p.NotInterested = append([]string(nil), p.NotInterested...)
	return p
}

func (s *Service) SaveRecommendationPreferences(p RecommendationPreferences) error {
	if !validRecommendationSort(p.DefaultSort) {
		return errInvalidRecommendationSort
	}
	p = sanitizeRecommendationPreferences(p)
	s.mu.Lock()
	previous := s.preferences
	if p.NotInterested == nil {
		p.NotInterested = append([]string(nil), previous.NotInterested...)
	}
	s.preferences = p
	err := storage.Save(s.recommendationPath, recommendationVersion, recommendationState{Preferences: p})
	if err != nil {
		s.preferences = previous
	}
	s.mu.Unlock()
	if err != nil {
		return uierr.Wrap("catalog.recommendation_save_failed", err)
	}
	return nil
}

// SetNotInterested is idempotent and persists before exposing the change in
// memory. The caller can pass false to undo the decision.
func (s *Service) SetNotInterested(gameID string, on bool) error {
	gameID = strings.TrimSpace(gameID)
	if gameID == "" {
		return errEmptyRecommendationID
	}
	s.mu.Lock()
	previous := s.preferences
	next := previous
	next.NotInterested = append([]string(nil), previous.NotInterested...)
	if on {
		found := false
		for _, id := range next.NotInterested {
			if id == gameID {
				found = true
				break
			}
		}
		if !found {
			next.NotInterested = append(next.NotInterested, gameID)
		}
	} else {
		kept := next.NotInterested[:0]
		for _, id := range next.NotInterested {
			if id != gameID {
				kept = append(kept, id)
			}
		}
		next.NotInterested = kept
	}
	next = sanitizeRecommendationPreferences(next)
	s.preferences = next
	err := storage.Save(s.recommendationPath, recommendationVersion, recommendationState{Preferences: next})
	if err != nil {
		s.preferences = previous
	}
	s.mu.Unlock()
	if err != nil {
		return uierr.Wrap("catalog.recommendation_save_failed", err)
	}
	return nil
}

func (s *Service) recommendationSnapshot() ([]Game, RecommendationPreferences, RecommendationLibrarySource) {
	s.mu.RLock()
	games := make([]Game, 0, len(s.games))
	for _, game := range s.games {
		if s.resolveIDLocked(game.ID) == game.ID {
			games = append(games, game)
		}
	}
	p := s.preferences
	p.NotInterested = append([]string(nil), p.NotInterested...)
	source := s.recommendationLibrary
	s.mu.RUnlock()
	return games, p, source
}

func (s *Service) GetRecommendationProfile() RecommendationProfile {
	games, p, source := s.recommendationSnapshot()
	items := []RecommendationLibraryItem(nil)
	if source != nil {
		items = source()
	}
	evidence := buildEvidence(games, items)
	return profileFromEvidence(evidence, p)
}

func profileFromEvidence(e recommendationEvidence, p RecommendationPreferences) RecommendationProfile {
	confidence := minFloat(1, float64(e.Games)/float64(personalizationMinGames))
	if e.Playtime > 0 {
		confidence = maxFloat(confidence, minFloat(1, float64(e.Playtime)/float64(personalizationMinPlaytime)))
	}
	defaultSort := "popular"
	if confidence >= 1 {
		defaultSort = "for-you"
	}
	return RecommendationProfile{DefaultSort: defaultSort, Confidence: confidence, EvidenceGames: e.Games, EvidencePlaytime: e.Playtime,
		Genres: topFacets(e.Genres), Themes: topFacets(e.Themes)}
}

func buildEvidence(games []Game, items []RecommendationLibraryItem) recommendationEvidence {
	byID := make(map[string]Game, len(games))
	for _, game := range games {
		byID[game.ID] = game
	}
	e := recommendationEvidence{Genres: map[string]float64{}, Themes: map[string]float64{}}
	for _, item := range items {
		if item.Hidden {
			continue
		}
		id := item.CanonicalGameID
		game, ok := byID[id]
		if !ok {
			game, ok = byID[item.LibraryID]
		}
		if !ok {
			continue
		}
		meaningful := item.Favorite || (item.Sessions >= 2 && item.PlaytimeSeconds >= minMeaningfulPlaytime)
		if !meaningful {
			continue
		}
		e.Games++
		e.Playtime += item.PlaytimeSeconds
		weight := 1.0 + minFloat(3, float64(item.PlaytimeSeconds)/float64(2*60*60))
		if item.Favorite {
			weight += 2
		}
		for _, genre := range game.Genres {
			e.Genres[genre] += weight
		}
		for _, theme := range game.Themes {
			e.Themes[theme] += weight
		}
	}
	return e
}

func topFacets(values map[string]float64) []RecommendationFacet {
	result := make([]RecommendationFacet, 0, len(values))
	for value, score := range values {
		result = append(result, RecommendationFacet{Value: value, Score: score})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Score != result[j].Score {
			return result[i].Score > result[j].Score
		}
		return result[i].Value < result[j].Value
	})
	if len(result) > 8 {
		result = result[:8]
	}
	return result
}

func (s *Service) GetDiscovery(q DiscoveryQuery) DiscoveryResult {
	games, p, source := s.recommendationSnapshot()
	items := []RecommendationLibraryItem(nil)
	if source != nil {
		items = source()
	}
	profile := profileFromEvidence(buildEvidence(games, items), p)
	limit := q.Limit
	if limit <= 0 || limit > maxDiscoveryItems {
		limit = maxDiscoveryItems
	}
	exclude := mergeIDs(q.RefreshExcludeIDs, q.ExcludeIDs)
	libraryIDs := libraryGameIDs(items)
	seen := map[string]bool{}
	for _, id := range p.NotInterested {
		seen[id] = true
	}
	for _, id := range exclude {
		seen[id] = true
	}
	for id := range libraryIDs {
		seen[id] = true
	}
	// Discovery is always outside the library. The explicit query flag is
	// accepted for API symmetry, but cannot make the discovery block leak owned
	// games into its own results.
	candidates := rankGames(games, profile, items, p, false, seen, q.GameQuery)
	selected := diverseRecommendations(candidates, limit)
	return DiscoveryResult{Items: selected, Fallback: profile.DefaultSort == "popular" && profile.EvidenceGames == 0, Profile: profile}
}

func (s *Service) GetLibraryRecommendations(q LibraryRecommendationQuery) []RecommendationItem {
	games, p, source := s.recommendationSnapshot()
	if source == nil {
		return []RecommendationItem{}
	}
	items := source()
	profile := profileFromEvidence(buildEvidence(games, items), p)
	excludeLibrary := make(map[string]bool, len(q.ExcludeLibraryIDs))
	for _, id := range q.ExcludeLibraryIDs {
		excludeLibrary[id] = true
	}
	excludeGames := make(map[string]bool, len(q.ExcludeGameIDs))
	for _, id := range q.ExcludeGameIDs {
		excludeGames[id] = true
	}
	blocked := map[string]bool{}
	for _, id := range p.NotInterested {
		blocked[id] = true
	}
	limit := q.Limit
	if limit <= 0 || limit > maxDiscoveryItems {
		limit = maxDiscoveryItems
	}
	result := make([]RecommendationItem, 0, limit)
	for _, game := range rankGames(games, profile, items, p, true, blocked, GameQuery{}) {
		if game.LibraryID == "" || excludeLibrary[game.LibraryID] || excludeGames[game.Game.ID] {
			continue
		}
		if game.Game.GameType != "" && isAddonType(game.Game.GameType) {
			continue
		}
		if len(excludeLibrary) == 0 && game.Reason == "continue" {
			// The continue-playing hero already owns this slot in the library UI.
			continue
		}
		result = append(result, game.RecommendationItem)
		if len(result) == limit {
			break
		}
	}
	return result
}

type rankedRecommendation struct {
	RecommendationItem
	score float64
	genre string
}

func rankGames(games []Game, profile RecommendationProfile, library []RecommendationLibraryItem, p RecommendationPreferences, includeLibrary bool, blocked map[string]bool, q GameQuery) []rankedRecommendation {
	byID := map[string]RecommendationLibraryItem{}
	for _, item := range library {
		id := item.CanonicalGameID
		if id == "" {
			id = item.LibraryID
		}
		if id != "" {
			byID[id] = item
		}
	}
	genreScores := map[string]float64{}
	for _, facet := range profile.Genres {
		genreScores[strings.ToLower(facet.Value)] = facet.Score
	}
	themeScores := map[string]float64{}
	for _, facet := range profile.Themes {
		themeScores[strings.ToLower(facet.Value)] = facet.Score
	}
	result := make([]rankedRecommendation, 0, len(games))
	for _, game := range games {
		if blocked[game.ID] || game.GameType != "" && isAddonType(game.GameType) {
			continue
		}
		if q.Genre != "" && !genreMatches(game.Genres, q.Genre) {
			continue
		}
		if q.Platform != "" && !containsFold(game.Platforms, q.Platform) {
			continue
		}
		if q.Kind != "" && !strings.EqualFold(strings.TrimSpace(game.GameType), strings.TrimSpace(q.Kind)) {
			continue
		}
		item, inLibrary := byID[game.ID]
		if !includeLibrary && inLibrary {
			continue
		}
		if includeLibrary && !inLibrary {
			continue
		}
		if item.Hidden || item.ContinuePlaying {
			continue
		}
		score, reason, title, reasonGenre := scoreRecommendation(game, profile, item, inLibrary, genreScores, themeScores)
		if reason == "similar" {
			title = similarReference(game, games, library)
		}
		result = append(result, rankedRecommendation{RecommendationItem: RecommendationItem{Game: game, LibraryID: item.LibraryID, Reason: reason, ReasonTitle: title, ReasonGenre: reasonGenre}, score: score, genre: reasonGenre})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].score != result[j].score {
			return result[i].score > result[j].score
		}
		return lessByTitle(result[i].Game, result[j].Game)
	})
	return result
}

func similarReference(candidate Game, games []Game, library []RecommendationLibraryItem) string {
	byID := make(map[string]RecommendationLibraryItem, len(library))
	for _, item := range library {
		id := item.CanonicalGameID
		if id == "" {
			id = item.LibraryID
		}
		byID[id] = item
	}
	for _, game := range games {
		if game.ID == candidate.ID {
			continue
		}
		item, ok := byID[game.ID]
		if !ok || item.Hidden || (!item.Favorite && (item.Sessions < 2 || item.PlaytimeSeconds < minMeaningfulPlaytime)) {
			continue
		}
		if shareValue(candidate.Genres, game.Genres) || shareValue(candidate.Themes, game.Themes) {
			return game.Title
		}
	}
	return ""
}

func shareValue(left, right []string) bool {
	for _, a := range left {
		for _, b := range right {
			if strings.EqualFold(a, b) {
				return true
			}
		}
	}
	return false
}

func scoreRecommendation(game Game, profile RecommendationProfile, item RecommendationLibraryItem, inLibrary bool, genres, themes map[string]float64) (float64, string, string, string) {
	genreScore, bestGenre := 0.0, ""
	for _, genre := range game.Genres {
		if score := genres[strings.ToLower(genre)]; score > genreScore {
			genreScore, bestGenre = score, genre
		}
	}
	themeScore := 0.0
	for _, theme := range game.Themes {
		themeScore = maxFloat(themeScore, themes[strings.ToLower(theme)])
	}
	quality := qualityScore(game)
	popularity := popularityScore(game)
	score := genreScore*0.55 + themeScore*0.15 + quality*0.2 + popularity*0.1
	if profile.DefaultSort == "popular" && profile.Confidence == 0 {
		score = quality*0.6 + popularity*0.4
	}
	if inLibrary {
		if item.Favorite {
			score += 0.15
		}
		if item.Installed {
			score += 0.05
		}
		if item.PlaytimeSeconds == 0 {
			return score + 0.08, "unplayed", "", bestGenre
		}
		if item.LastPlayed != nil && time.Since(*item.LastPlayed) > 90*24*time.Hour {
			return score + 0.04, "return", "", bestGenre
		}
		return score, "similar", "", bestGenre
	}
	if bestGenre != "" && genreScore > 0 {
		return score, "genre", "", bestGenre
	}
	if quality > 0 || popularity > 0 {
		return score, "popular", "", ""
	}
	return score, "similar", "", ""
}

func diverseRecommendations(candidates []rankedRecommendation, limit int) []RecommendationItem {
	selected := make([]RecommendationItem, 0, minInt(limit, len(candidates)))
	usedGenres := map[string]bool{}
	used := map[string]bool{}
	for pass := 0; pass < 2 && len(selected) < limit; pass++ {
		for _, candidate := range candidates {
			if used[candidate.Game.ID] || (pass == 0 && candidate.genre != "" && usedGenres[strings.ToLower(candidate.genre)]) {
				continue
			}
			used[candidate.Game.ID] = true
			if candidate.genre != "" {
				usedGenres[strings.ToLower(candidate.genre)] = true
			}
			selected = append(selected, candidate.RecommendationItem)
			if len(selected) == limit {
				break
			}
		}
	}
	return selected
}

func libraryGameIDs(items []RecommendationLibraryItem) map[string]bool {
	ids := map[string]bool{}
	for _, item := range items {
		if item.Hidden {
			continue
		}
		if item.CanonicalGameID != "" {
			ids[item.CanonicalGameID] = true
		}
		if item.LibraryID != "" {
			ids[item.LibraryID] = true
		}
	}
	return ids
}

func filterRecommendationGames(games []Game, q GameQuery, blocked map[string]bool, hideNotInterested bool) []Game {
	result := make([]Game, 0, len(games))
	for _, game := range games {
		if hideNotInterested && blocked[game.ID] {
			continue
		}
		if q.Genre != "" && !genreMatches(game.Genres, q.Genre) {
			continue
		}
		result = append(result, game)
	}
	return result
}

func qualityScore(game Game) float64 {
	reviewCount := game.ReviewCount
	if reviewCount == nil {
		reviewCount = game.RatingCount
	}
	if game.Rating == nil || reviewCount == nil || *reviewCount <= 0 {
		return 0
	}
	rating := minFloat(10, maxFloat(0, *game.Rating)) / 10
	confidence := float64(*reviewCount) / float64(*reviewCount+100)
	return rating * confidence
}

func popularityScore(game Game) float64 {
	if game.ExternalPopularity == nil {
		return 0
	}
	return minFloat(1, maxFloat(0, *game.ExternalPopularity))
}

func isAddonType(kind string) bool {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "dlc", "demo", "soundtrack", "expansion", "addon", "add-on":
		return true
	default:
		return false
	}
}

func mergeIDs(groups ...[]string) []string {
	seen := map[string]bool{}
	result := []string{}
	for _, group := range groups {
		for _, id := range group {
			id = strings.TrimSpace(id)
			if id != "" && !seen[id] {
				seen[id] = true
				result = append(result, id)
			}
		}
	}
	return result
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func containsFold(values []string, want string) bool {
	for _, value := range values {
		if strings.EqualFold(value, want) {
			return true
		}
	}
	return false
}
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

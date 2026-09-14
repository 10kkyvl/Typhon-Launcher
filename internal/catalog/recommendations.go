package catalog

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"typhon/internal/storage"
	"typhon/internal/uierr"
)

const (
	recommendationVersion = 1
	maxDiscoveryItems     = 5
)

// RecommendationConfig keeps the first ranking policy explicit and tunable.
// TYPHON_RECOMMENDATION_CONFIG accepts a JSON object with these fields; an
// invalid object or an out-of-range value uses all defaults. The environment
// is read once when the process starts, so one catalog query cannot change
// ranking semantics halfway through a session.
type RecommendationConfig struct {
	MinMeaningfulSeconds           int64   `json:"minMeaningfulSeconds"`
	MeaningfulSessions             int     `json:"meaningfulSessions"`
	PersonalizationMinGames        int     `json:"personalizationMinGames"`
	PersonalizationMinPlaytimeSecs int64   `json:"personalizationMinPlaytimeSeconds"`
	ReturnDays                     int     `json:"returnDays"`
	InstalledBoost                 float64 `json:"installedBoost"`
	FavoriteBoost                  float64 `json:"favoriteBoost"`
	GenreWeight                    float64 `json:"genreWeight"`
	ThemeWeight                    float64 `json:"themeWeight"`
}

func defaultRecommendationConfig() RecommendationConfig {
	return RecommendationConfig{
		MinMeaningfulSeconds:           30 * 60,
		MeaningfulSessions:             2,
		PersonalizationMinGames:        3,
		PersonalizationMinPlaytimeSecs: 3 * 60 * 60,
		ReturnDays:                     90,
		InstalledBoost:                 0.05,
		FavoriteBoost:                  0.15,
		GenreWeight:                    0.55,
		ThemeWeight:                    0.15,
	}
}

func parseRecommendationConfig(raw string) (RecommendationConfig, error) {
	config := defaultRecommendationConfig()
	if strings.TrimSpace(raw) == "" {
		return config, nil
	}
	if err := json.Unmarshal([]byte(raw), &config); err != nil {
		return defaultRecommendationConfig(), err
	}
	if config.MinMeaningfulSeconds < 60 || config.MinMeaningfulSeconds > 24*60*60 ||
		config.MeaningfulSessions < 2 || config.MeaningfulSessions > 20 ||
		config.PersonalizationMinGames < 1 || config.PersonalizationMinGames > 100 ||
		config.PersonalizationMinPlaytimeSecs < 60 || config.PersonalizationMinPlaytimeSecs > 30*24*60*60 ||
		config.ReturnDays < 1 || config.ReturnDays > 3650 ||
		config.InstalledBoost < 0 || config.InstalledBoost > 1 ||
		config.FavoriteBoost < 0 || config.FavoriteBoost > 1 ||
		config.GenreWeight < 0 || config.GenreWeight > 1 ||
		config.ThemeWeight < 0 || config.ThemeWeight > 1 {
		return defaultRecommendationConfig(), errors.New("recommendation config value out of range")
	}
	return config, nil
}

var recommendationConfig = func() RecommendationConfig {
	config, err := parseRecommendationConfig(os.Getenv("TYPHON_RECOMMENDATION_CONFIG"))
	if err != nil {
		return defaultRecommendationConfig()
	}
	return config
}()

var (
	errInvalidRecommendationSort  = uierr.New("catalog.invalid_recommendation_sort", "неизвестная сортировка каталога")
	errEmptyRecommendationID      = uierr.New("catalog.empty_recommendation_id", "не указан идентификатор игры")
	errRecommendationsUnavailable = errors.New("recommendation library unavailable")
)

// RecommendationLibraryItem is the small, stable boundary between catalog
// ranking and the launcher library. The catalog deliberately does not import
// library: main wires this snapshot callback after both services start.
type RecommendationLibraryItem struct {
	Title           string     `json:"title,omitempty"`
	Cover           string     `json:"cover,omitempty"`
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
	_, p, source, items, profile := s.recommendationSnapshotWithProfile()
	return s.enrichRecommendationQueryWithSnapshot(q, p, source, items, profile)
}

func (s *Service) enrichRecommendationQueryWithSnapshot(q GameQuery, p RecommendationPreferences, source RecommendationLibrarySource, items []RecommendationLibraryItem, profile RecommendationProfile) GameQuery {
	if q.Sort == "for-you" && strings.TrimSpace(q.Profile) == "" {
		weights := make(map[string]float64, len(profile.Genres))
		maxWeight := 0.0
		for _, facet := range profile.Genres {
			maxWeight = maxFloat(maxWeight, facet.Score)
		}
		themes := make(map[string]float64, len(profile.Themes))
		for _, facet := range profile.Themes {
			maxWeight = maxFloat(maxWeight, facet.Score)
		}
		if maxWeight == 0 {
			maxWeight = 1
		}
		for _, facet := range profile.Genres {
			weights[facet.Value] = facet.Score / maxWeight
		}
		for _, facet := range profile.Themes {
			themes[facet.Value] = facet.Score / maxWeight
		}
		payload, _ := json.Marshal(struct {
			Genres   map[string]float64 `json:"genres,omitempty"`
			Themes   map[string]float64 `json:"themes,omitempty"`
			Strength float64            `json:"strength,omitempty"`
		}{Genres: weights, Themes: themes, Strength: profile.Confidence})
		q.Profile = string(payload)
	}
	ids := append([]string(nil), q.ExcludeIDs...)
	if q.HideNotInterested {
		ids = append(ids, p.NotInterested...)
	}
	q.ExcludeNotInterested = strings.Join(s.remoteRecommendationIDs(ids), ",")
	if q.HideLibrary && source != nil {
		ids := make([]string, 0, len(items))
		for _, item := range items {
			if item.CanonicalGameID != "" {
				ids = append(ids, item.CanonicalGameID)
			}
		}
		q.ExcludeLibrary = strings.Join(s.remoteRecommendationIDs(ids), ",")
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
	s.mu.Lock()
	if s.recommendationLoadErr != nil {
		s.mu.Unlock()
		return uierr.Wrap("catalog.recommendation_save_failed", s.recommendationLoadErr)
	}
	previous := s.preferences
	// Dismissals have their own mutation API. Keeping the stored list here
	// prevents a stale preferences response from undoing a newer decision.
	p.NotInterested = append([]string(nil), previous.NotInterested...)
	p = sanitizeRecommendationPreferences(p)
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

func (s *Service) recommendationCatalogIDLocked(id string) string {
	if id == "" {
		return ""
	}
	canonical := s.resolveIDLocked(id)
	if canonical != "" && canonical != id {
		return canonical
	}
	for _, game := range s.games {
		if game.ID == id || game.ServerID == id {
			return game.ID
		}
	}
	return id
}

// SetNotInterested is idempotent and persists before exposing the change in
// memory. The caller can pass false to undo the decision.
func (s *Service) SetNotInterested(gameID string, on bool) error {
	gameID = strings.TrimSpace(gameID)
	if gameID == "" {
		return errEmptyRecommendationID
	}
	s.mu.Lock()
	if s.recommendationLoadErr != nil {
		err := s.recommendationLoadErr
		s.mu.Unlock()
		return uierr.Wrap("catalog.recommendation_save_failed", err)
	}
	if canonical := s.recommendationCatalogIDLocked(gameID); canonical != "" {
		gameID = canonical
	}
	previous := s.preferences
	next := previous
	next.NotInterested = append([]string(nil), previous.NotInterested...)
	if on {
		found := false
		for _, id := range next.NotInterested {
			if s.recommendationCatalogIDLocked(id) == gameID {
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
			if s.recommendationCatalogIDLocked(id) != gameID {
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
	original := s.recommendationLibrary
	aliases := make(map[string]string, len(s.games)*2)
	for _, g := range s.games {
		aliases[g.ID] = s.resolveIDLocked(g.ID)
		if g.ServerID != "" {
			aliases[g.ServerID] = s.resolveIDLocked(g.ID)
		}
	}
	for i, id := range p.NotInterested {
		if canonical := aliases[id]; canonical != "" {
			p.NotInterested[i] = canonical
		}
	}
	s.mu.RUnlock()
	var source RecommendationLibrarySource
	if original != nil {
		source = func() []RecommendationLibraryItem {
			items := append([]RecommendationLibraryItem(nil), original()...)
			for i := range items {
				if id := aliases[items[i].CanonicalGameID]; id != "" {
					items[i].CanonicalGameID = id
				}
			}
			return items
		}
	}
	return games, p, source
}

// recommendationSnapshotWithProfile takes one coherent snapshot of catalog
// and library evidence. A Browse request may need the profile and the same
// library IDs for exclusions; calling the source separately can rescan the
// whole library twice and observe two different states.
func (s *Service) recommendationSnapshotWithProfile() ([]Game, RecommendationPreferences, RecommendationLibrarySource, []RecommendationLibraryItem, RecommendationProfile) {
	games, p, source := s.recommendationSnapshot()
	items := []RecommendationLibraryItem(nil)
	if source != nil {
		items = source()
	}
	profile := profileFromEvidence(buildEvidence(games, filterEvidenceItems(items, p)), p)
	if s.recommendationLoadErr != nil {
		profile = profileFromEvidence(recommendationEvidence{}, p)
	}
	return games, p, source, items, profile
}

func (s *Service) GetRecommendationProfile() RecommendationProfile {
	if s.recommendationLoadErr != nil {
		return profileFromEvidence(recommendationEvidence{}, RecommendationPreferences{})
	}
	_, _, _, _, profile := s.recommendationSnapshotWithProfile()
	return profile
}

func profileFromEvidence(e recommendationEvidence, p RecommendationPreferences) RecommendationProfile {
	confidence := minFloat(1, float64(e.Games)/float64(recommendationConfig.PersonalizationMinGames))
	if e.Playtime > 0 {
		confidence = maxFloat(confidence, minFloat(1, float64(e.Playtime)/float64(recommendationConfig.PersonalizationMinPlaytimeSecs)))
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
	// Several releases or synced copies of one game are one preference signal.
	// Maxima avoid multiplying counters copied between library entries.
	unique := make(map[string]RecommendationLibraryItem, len(items))
	for _, item := range items {
		if item.Hidden {
			continue
		}
		id := item.CanonicalGameID
		if _, ok := byID[id]; !ok {
			id = item.LibraryID
		}
		previous := unique[id]
		item.CanonicalGameID = id
		item.Favorite = item.Favorite || previous.Favorite
		item.Sessions = max(item.Sessions, previous.Sessions)
		item.PlaytimeSeconds = max(item.PlaytimeSeconds, previous.PlaytimeSeconds)
		unique[id] = item
	}
	ids := make([]string, 0, len(unique))
	for id := range unique {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		item := unique[id]
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
		meaningful := item.Favorite || (item.Sessions >= recommendationConfig.MeaningfulSessions && item.PlaytimeSeconds >= recommendationConfig.MinMeaningfulSeconds)
		if !meaningful {
			continue
		}
		e.Games++
		e.Playtime += item.PlaytimeSeconds
		weight := 1.0 + minFloat(3, float64(item.PlaytimeSeconds)/float64(2*60*60))
		if item.Favorite {
			weight += 2
		}
		for _, genre := range canonicalGenres(game.Genres) {
			e.Genres[genre] += weight
		}
		for _, theme := range game.Themes {
			e.Themes[theme] += weight
		}
	}
	return e
}

func filterEvidenceItems(items []RecommendationLibraryItem, p RecommendationPreferences) []RecommendationLibraryItem {
	blocked := make(map[string]bool, len(p.NotInterested))
	for _, id := range p.NotInterested {
		blocked[id] = true
	}
	result := make([]RecommendationLibraryItem, 0, len(items))
	for _, item := range items {
		if blocked[item.CanonicalGameID] || blocked[item.LibraryID] {
			continue
		}
		result = append(result, item)
	}
	return result
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
	games, p, _, items, profile := s.recommendationSnapshotWithProfile()
	limit := q.Limit
	if limit <= 0 || limit > maxDiscoveryItems {
		limit = maxDiscoveryItems
	}
	libraryIDs := libraryGameIDs(items)
	baseSeen := map[string]bool{}
	for _, id := range p.NotInterested {
		baseSeen[id] = true
	}
	for _, id := range q.ExcludeIDs {
		baseSeen[id] = true
	}
	for id := range libraryIDs {
		baseSeen[id] = true
	}
	seen := cloneBoolMap(baseSeen)
	for _, id := range q.RefreshExcludeIDs {
		seen[id] = true
	}
	// Discovery is always outside the library. The explicit query flag is
	// accepted for API symmetry, but cannot make the discovery block leak owned
	// games into its own results.
	remoteQuery := q.GameQuery
	// Fetch one remote candidate stream. Refresh exclusions are applied while
	// ranking so the same stream can provide both fresh picks and the existing
	// picks needed to fill a short shelf.
	remoteQuery.HideLibrary, remoteQuery.HideNotInterested = true, true
	sortName := "popular"
	if profile.Confidence > 0 {
		sortName = "for-you"
	}
	candidateGames, remoteOK := s.remoteDiscoveryGames(remoteQuery, sortName, func(candidateGames []Game) bool {
		candidates := rankGames(candidateGames, profile, items, p, false, seen, q.GameQuery, games)
		return len(diverseRecommendations(candidates, limit)) >= limit
	})
	if !remoteOK {
		candidateGames = games
		if q.Compat == CompatOnlyWorking {
			candidateGames = make([]Game, 0, len(games))
			s.mu.RLock()
			for _, game := range games {
				if s.compatWorksLocked(game.ID) {
					candidateGames = append(candidateGames, game)
				}
			}
			s.mu.RUnlock()
		}
	}
	candidates := rankGames(candidateGames, profile, items, p, false, seen, q.GameQuery, games)
	selected := diverseRecommendations(candidates, limit)
	// Prefer fresh picks, but retain enough existing eligible picks when there
	// are fewer alternatives than shelf slots. Reuse the same remote candidate
	// stream instead of issuing a second discovery request for the backfill.
	if len(selected) < limit && len(q.RefreshExcludeIDs) > 0 {
		oldCandidates := rankGames(candidateGames, profile, items, p, false, baseSeen, q.GameQuery, games)
		old := diverseRecommendations(oldCandidates, limit)
		used := map[string]bool{}
		for _, item := range selected {
			used[item.Game.ID] = true
		}
		for _, item := range old {
			if !used[item.Game.ID] {
				selected = append(selected, item)
				used[item.Game.ID] = true
			}
			if len(selected) == limit {
				break
			}
		}
	}
	s.rememberDiscoveryGames(selected)
	return DiscoveryResult{Items: selected, Fallback: !remoteOK || s.recommendationLoadErr != nil, Profile: profile}
}

func (s *Service) remoteDiscoveryGames(q GameQuery, sortName string, enough func([]Game) bool) ([]Game, bool) {
	q.Sort = sortName
	q.Page = 1
	q.PageSize = 60
	page, err := s.browseGames(q, false)
	if err != nil || page.Offline {
		return nil, false
	}
	defer func() {
		s.mu.Lock()
		delete(s.browseSnapshots, page.Snapshot)
		s.mu.Unlock()
	}()
	items := append([]Game(nil), page.Items...)
	for pageNumber := 2; pageNumber <= 3 && !enough(items) && len(page.Items) >= q.PageSize; pageNumber++ {
		continuation := q
		continuation.Page = pageNumber
		continuation.Snapshot = page.Snapshot
		continuation.Revision = page.Revision
		continuation.PageSize = q.PageSize
		next, nextErr := s.browseGames(continuation, false)
		if nextErr != nil || next.Offline {
			// The first page is still a valid, frozen result. Do not replace it
			// with a different revision or turn a continuation failure into a
			// general-catalog fallback.
			break
		}
		items = append(items, next.Items...)
		page = next
	}
	return items, true
}

func (s *Service) GetLibraryRecommendations(q LibraryRecommendationQuery) ([]RecommendationItem, error) {
	if s.recommendationLoadErr != nil {
		return nil, fmt.Errorf("recommendation preferences unavailable: %w", s.recommendationLoadErr)
	}
	games, p, source := s.recommendationSnapshot()
	if source == nil {
		return nil, errRecommendationsUnavailable
	}
	items := source()
	known := map[string]bool{}
	for _, g := range games {
		known[g.ID] = true
	}
	for _, item := range items {
		id := item.CanonicalGameID
		if id == "" {
			id = item.LibraryID
		}
		if id != "" && !known[id] && item.Title != "" {
			games = append(games, Game{ID: id, Title: item.Title, SortTitle: strings.ToLower(item.Title), CoverURL: item.Cover})
			known[id] = true
		}
	}
	profile := profileFromEvidence(buildEvidence(games, filterEvidenceItems(items, p)), p)
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
	return result, nil
}

type rankedRecommendation struct {
	RecommendationItem
	score float64
	genre string
}

func rankGames(games []Game, profile RecommendationProfile, library []RecommendationLibraryItem, p RecommendationPreferences, includeLibrary bool, blocked map[string]bool, q GameQuery, references ...[]Game) []rankedRecommendation {
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
		genreScores[strings.ToLower(canonicalGenre(facet.Value))] += facet.Score
	}
	themeScores := map[string]float64{}
	for _, facet := range profile.Themes {
		themeScores[strings.ToLower(facet.Value)] = facet.Score
	}
	result := make([]rankedRecommendation, 0, len(games))
	for _, game := range games {
		if blocked[game.ID] || !contentKindMatches(q.Kind, game.GameType) {
			continue
		}
		if q.Genre != "" && !genreMatches(game.Genres, q.Genre) {
			continue
		}
		if q.Platform != "" && !containsFold(game.Platforms, q.Platform) {
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
		if includeLibrary && item.LastPlayed != nil && time.Since(*item.LastPlayed) > time.Duration(recommendationConfig.ReturnDays)*24*time.Hour && !hasReturnSignal(game, item, genreScores, themeScores) {
			// A short, abandoned launch is not enough evidence to ask the
			// user to return. Keep stale games only when history or affinity
			// gives us a concrete reason.
			continue
		}
		score, reason, title, reasonGenre := scoreRecommendation(game, profile, item, inLibrary, genreScores, themeScores)
		if reason == "" {
			continue
		}
		if reason == "similar" {
			referenceGames := games
			if len(references) > 0 {
				referenceGames = references[0]
			}
			title = similarReference(game, referenceGames, library)
			if title == "" {
				continue
			}
		}
		result = append(result, rankedRecommendation{RecommendationItem: RecommendationItem{Game: game, LibraryID: item.LibraryID, Reason: reason, ReasonTitle: title, ReasonGenre: reasonGenre}, score: score, genre: recommendationPrimaryGenre(game, reasonGenre)})
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
		if !ok || item.Hidden || (!item.Favorite && (item.Sessions < recommendationConfig.MeaningfulSessions || item.PlaytimeSeconds < recommendationConfig.MinMeaningfulSeconds)) {
			continue
		}
		if shareValue(canonicalGenres(candidate.Genres), canonicalGenres(game.Genres)) || shareValue(candidate.Themes, game.Themes) {
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
	for _, genre := range canonicalGenres(game.Genres) {
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
	score := genreScore*recommendationConfig.GenreWeight + themeScore*recommendationConfig.ThemeWeight + quality*0.2 + popularity*0.1
	if profile.DefaultSort == "popular" && profile.Confidence == 0 {
		score = quality*0.6 + popularity*0.4
	}
	if inLibrary {
		if item.Favorite {
			score += recommendationConfig.FavoriteBoost
		}
		if item.Installed {
			score += recommendationConfig.InstalledBoost
		}
		if item.Sessions == 0 && item.PlaytimeSeconds == 0 && item.LastPlayed == nil {
			return score + 0.08, "unplayed", "", bestGenre
		}
		if item.LastPlayed != nil && time.Since(*item.LastPlayed) > time.Duration(recommendationConfig.ReturnDays)*24*time.Hour && hasReturnSignal(game, item, genres, themes) {
			return score + 0.04, "return", "", bestGenre
		}
		if item.Favorite {
			return score, "favorite", "", bestGenre
		}
		return score, "similar", "", bestGenre
	}
	if themeScore > 0 && bestGenre == "" {
		return score, "similar", "", ""
	}
	if bestGenre != "" && genreScore > 0 {
		return score, "genre", "", bestGenre
	}
	if quality > 0 || popularity > 0 {
		return score, "popular", "", ""
	}
	if len(game.Genres) > 0 {
		return score, "category", "", game.Genres[0]
	}
	return score, "", "", ""
}

func hasReturnSignal(game Game, item RecommendationLibraryItem, genres, themes map[string]float64) bool {
	if item.Favorite || (item.Sessions >= recommendationConfig.MeaningfulSessions && item.PlaytimeSeconds >= recommendationConfig.MinMeaningfulSeconds) {
		return true
	}
	for _, genre := range canonicalGenres(game.Genres) {
		if genres[strings.ToLower(genre)] > 0 {
			return true
		}
	}
	for _, theme := range game.Themes {
		if themes[strings.ToLower(theme)] > 0 {
			return true
		}
	}
	return false
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
	if game.Rating == nil || game.RatingCount == nil || *game.RatingCount <= 0 {
		return 0
	}
	rating := minFloat(100, maxFloat(0, *game.Rating)) / 100
	confidence := float64(*game.RatingCount) / float64(*game.RatingCount+100)
	return rating * confidence
}

func popularityScore(game Game) float64 {
	if game.Rating == nil || game.RatingCount == nil || *game.RatingCount <= 0 {
		return 0
	}
	// Mirror the backend's bounded recommendation signal: use the actual
	// provider rating count, log-scaled, with confidence-adjusted rating.
	raw := math.Log1p(float64(*game.RatingCount)) * qualityScore(game)
	return minFloat(1, maxFloat(0, raw/5))
}

func isAddonType(kind string) bool {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "dlc", "dlc / addon", "demo", "soundtrack", "music", "expansion", "addon", "add-on", "bundle", "edition":
		return true
	default:
		return false
	}
}

// contentKindMatches mirrors the backend catalog groups. Empty and "game"
// hide known non-game/add-on types while retaining records with an unknown
// type. "all" includes known add-ons, but still omits software-like records
// which are not game catalog content.
func contentKindMatches(filter, gameType string) bool {
	filter = strings.ToLower(strings.TrimSpace(filter))
	kind := strings.ToLower(strings.TrimSpace(gameType))
	switch filter {
	case "", "game":
		return !isKnownCatalogExcludedType(kind)
	case "all":
		return !isNonGameType(kind)
	case "dlc":
		return kind == "dlc" || kind == "dlc / addon" || kind == "expansion"
	case "demo":
		return kind == "demo"
	case "soundtrack":
		return kind == "soundtrack" || kind == "music"
	case "bundle":
		return kind == "bundle"
	case "edition":
		return kind == "edition"
	default:
		return kind == filter
	}
}

func isKnownCatalogExcludedType(kind string) bool {
	return isAddonType(kind) || isNonGameType(kind)
}

func isNonGameType(kind string) bool {
	switch kind {
	case "software", "application", "video", "hardware", "tool", "mod":
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

func cloneBoolMap(values map[string]bool) map[string]bool {
	clone := make(map[string]bool, len(values))
	for key, value := range values {
		clone[key] = value
	}
	return clone
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

func recommendationPrimaryGenre(game Game, preferred string) string {
	if preferred != "" {
		return preferred
	}
	if len(game.Genres) > 0 {
		return game.Genres[0]
	}
	return ""
}

// Exclusions use canonical remote identities or explicit provider evidence.
// A local file-only game without either cannot be present in the public catalog.
func (s *Service) remoteRecommendationIDs(ids []string) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		resolved := s.resolveIDLocked(id)
		g, ok := s.idx.game(resolved)
		if ok {
			if server, err := uuid.Parse(g.ServerID); err == nil {
				out = append(out, server.String())
				continue
			}
			if value, err := strconv.ParseInt(g.ExternalIDs.IGDB, 10, 64); err == nil && value > 0 {
				out = append(out, "igdb:"+strconv.FormatInt(value, 10))
				continue
			}
			if value, err := strconv.ParseInt(g.ExternalIDs.Steam, 10, 64); err == nil && value > 0 {
				out = append(out, "steam:"+strconv.FormatInt(value, 10))
				continue
			}
		}
		if parsed, err := uuid.Parse(resolved); err == nil {
			out = append(out, parsed.String())
		}
	}
	return mergeIDs(out)
}

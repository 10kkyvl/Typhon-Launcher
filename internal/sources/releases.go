package sources

import (
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"typhon/internal/catalog"
	"typhon/internal/titles"
	"typhon/internal/uierr"
	"typhon/internal/version"
)

const (
	defaultPageSize = 50
	maxPageSize     = 200
)

var (
	errReleaseNotFound = uierr.New("sources.release_not_found", "релиз не найден")
	errNoURI           = uierr.New("sources.no_uri", "у релиза нет ссылки для загрузки")
	errNoCatalog       = uierr.New("sources.catalog_unavailable", "каталог игр недоступен")
)

func (s *Service) QueryReleases(q ReleaseQuery) ReleasePage {
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
	normalizedSearch := titles.Normalize(search)

	s.mu.Lock()
	defer s.mu.Unlock()

	var filtered []*Release
	for sourceID, list := range s.releases {
		if q.SourceID != "" && sourceID != q.SourceID {
			continue
		}
		for _, r := range list {
			if !matchesStatus(r, q.Status) {
				continue
			}
			if search != "" && !s.matchesSearch(r, search, normalizedSearch) {
				continue
			}
			filtered = append(filtered, r)
		}
	}
	sortReleases(filtered, q.Sort)

	total := len(filtered)
	start := (q.Page - 1) * q.PageSize
	if start > total {
		start = total
	}
	end := start + q.PageSize
	if end > total {
		end = total
	}
	items := make([]ReleaseView, 0, end-start)
	for _, r := range filtered[start:end] {
		items = append(items, s.viewLocked(r))
	}
	return ReleasePage{Items: items, Total: total, Page: q.Page, PageSize: q.PageSize}
}

func (s *Service) viewLocked(r *Release) ReleaseView {
	view := ReleaseView{Release: *r}
	if src := s.findLocked(r.SourceID); src != nil {
		view.SourceName = src.Name
	}
	if r.CanonicalGameID != nil && s.catalog != nil {
		view.GameTitle = s.catalog.TitleOf(*r.CanonicalGameID)
	}
	return view
}

func matchesStatus(r *Release, status string) bool {
	switch status {
	case "", "all":
		return !r.Ignored
	case "matched":
		return !r.Ignored && r.MatchStatus == catalog.StatusMatched && r.Availability == AvailabilityAvailable
	case "review":
		return !r.Ignored && r.MatchStatus == catalog.StatusReview && r.Availability == AvailabilityAvailable
	case "unmatched":
		return !r.Ignored && r.MatchStatus == catalog.StatusUnmatched && r.Availability == AvailabilityAvailable
	case "removed":
		return r.Availability == AvailabilityRemoved
	case "new":
		return r.New && !r.Ignored
	case "ignored":
		return r.Ignored
	default:
		return !r.Ignored
	}
}

func (s *Service) matchesSearch(r *Release, search, normalizedSearch string) bool {
	if releaseMatchesQuery(r, search, normalizedSearch) {
		return true
	}
	if r.CanonicalGameID != nil && s.catalog != nil {
		if title := s.catalog.TitleOf(*r.CanonicalGameID); title != "" {
			return strings.Contains(strings.ToLower(title), search)
		}
	}
	return false
}

func sortReleases(list []*Release, mode string) {
	switch mode {
	case "title":
		sort.Slice(list, func(a, b int) bool {
			if list[a].RawTitle != list[b].RawTitle {
				return list[a].RawTitle < list[b].RawTitle
			}
			return list[a].ID < list[b].ID
		})
	case "size":
		sort.Slice(list, func(a, b int) bool {
			if list[a].Size != list[b].Size {
				return list[a].Size > list[b].Size
			}
			return list[a].ID < list[b].ID
		})
	default:
		sort.Slice(list, func(a, b int) bool {
			left, right := list[a], list[b]
			switch {
			case left.UploadedAt == nil && right.UploadedAt == nil:
				if !left.FirstSeenAt.Equal(right.FirstSeenAt) {
					return left.FirstSeenAt.After(right.FirstSeenAt)
				}
			case left.UploadedAt == nil:
				return false
			case right.UploadedAt == nil:
				return true
			case !left.UploadedAt.Equal(*right.UploadedAt):
				return left.UploadedAt.After(*right.UploadedAt)
			}
			if left.RawTitle != right.RawTitle {
				return left.RawTitle < right.RawTitle
			}
			return left.ID < right.ID
		})
	}
}

func (s *Service) GetReleasesForGame(gameID string) []ReleaseGroup {
	if gameID == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	var list []*Release
	for _, releases := range s.releases {
		for _, r := range releases {
			if r.Ignored || r.CanonicalGameID == nil || *r.CanonicalGameID != gameID {
				continue
			}
			list = append(list, r)
		}
	}
	sortReleases(list, "")
	return s.groupLocked(list)
}

func (s *Service) GetReleasesForTitle(title string) []ReleaseGroup {
	if s.catalog == nil {
		return nil
	}
	game, ok := s.catalog.LookupByTitle(title)
	if !ok {
		return nil
	}
	return s.GetReleasesForGame(game.ID)
}

func (s *Service) groupLocked(list []*Release) []ReleaseGroup {
	groups := make([]ReleaseGroup, 0, len(list))
	byHash := map[string]int{}
	for _, r := range list {
		if r.InfoHash != "" {
			if pos, ok := byHash[r.InfoHash]; ok {
				groups[pos].Duplicates = append(groups[pos].Duplicates, SourceRef{
					ReleaseID:  r.ID,
					SourceID:   r.SourceID,
					SourceName: s.sourceNameLocked(r.SourceID),
				})
				continue
			}
			byHash[r.InfoHash] = len(groups)
		}
		groups = append(groups, ReleaseGroup{Release: *r, SourceName: s.sourceNameLocked(r.SourceID)})
	}
	return groups
}

func (s *Service) sourceNameLocked(id string) string {
	if src := s.findLocked(id); src != nil {
		return src.Name
	}
	return ""
}

func (s *Service) GetRelease(id string) (ReleaseView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r := s.findReleaseLocked(id)
	if r == nil {
		return ReleaseView{}, errReleaseNotFound
	}
	return s.viewLocked(r), nil
}

func (s *Service) findReleaseLocked(id string) *Release {
	for _, list := range s.releases {
		for _, r := range list {
			if r.ID == id {
				return r
			}
		}
	}
	return nil
}

func (s *Service) GetCandidates(releaseID string) ([]catalog.Candidate, error) {
	s.mu.Lock()
	r := s.findReleaseLocked(releaseID)
	if r == nil {
		s.mu.Unlock()
		return nil, errReleaseNotFound
	}
	query := catalog.Query{Title: r.Title, Normalized: r.NormalizedTitle, Year: r.Year}
	cat := s.catalog
	s.mu.Unlock()

	if cat == nil {
		return nil, errNoCatalog
	}
	return cat.Resolve(query).Candidates, nil
}

func (s *Service) ConfirmMatch(releaseID, gameID string) error {
	s.mu.Lock()
	r := s.findReleaseLocked(releaseID)
	if r == nil {
		s.mu.Unlock()
		return errReleaseNotFound
	}
	cat := s.catalog
	normalized := r.NormalizedTitle
	s.mu.Unlock()

	if cat == nil {
		return errNoCatalog
	}
	if _, err := cat.GetGame(gameID); err != nil {
		return err
	}
	if err := cat.LearnMatch(normalized, gameID); err != nil {
		return err
	}

	s.mu.Lock()
	touched := map[string]bool{}
	before := map[string][]Release{}
	for sourceID, list := range s.releases {
		for _, item := range list {
			if item.ID != releaseID && (item.Locked || item.NormalizedTitle != normalized) {
				continue
			}
			if !touched[sourceID] {
				before[sourceID] = snapshotReleases(list)
			}
			id := gameID
			item.CanonicalGameID = &id
			item.MatchStatus = catalog.StatusMatched
			item.MatchConfidence = 1
			item.MatchMethod = string(catalog.MethodOverride)
			if item.ID == releaseID {
				item.Locked = true
				item.MatchMethod = "manual"
			}
			touched[sourceID] = true
		}
	}
	if err := s.persistTouchedLocked(touched, before); err != nil {
		s.markDegradedLocked(err)
		s.mu.Unlock()
		return fmt.Errorf("save matched releases: %w", err)
	}
	s.clearDegradedLocked()
	if err := s.recountLocked(touched); err != nil {
		s.markDegradedLocked(err)
		s.mu.Unlock()
		return fmt.Errorf("recount sources: %w", err)
	}
	s.clearDegradedLocked()
	snapshots := s.snapshotsLocked(touched)
	s.mu.Unlock()

	slog.Info("release matched manually", "release", releaseID, "game", gameID, "pattern", normalized)
	for _, snapshot := range snapshots {
		emit(eventUpdated, snapshot)
	}
	emit(eventReleaseMatched, ReleaseBatch{Count: len(touched)})
	return nil
}

func (s *Service) IgnoreRelease(releaseID string, ignored bool) error {
	s.mu.Lock()
	r := s.findReleaseLocked(releaseID)
	if r == nil {
		s.mu.Unlock()
		return errReleaseNotFound
	}
	before := map[string][]Release{r.SourceID: snapshotReleases(s.releases[r.SourceID])}
	r.Ignored = ignored
	r.New = false
	touched := map[string]bool{r.SourceID: true}
	if err := s.persistTouchedLocked(touched, before); err != nil {
		s.markDegradedLocked(err)
		s.mu.Unlock()
		return fmt.Errorf("save ignored release: %w", err)
	}
	s.clearDegradedLocked()
	if err := s.recountLocked(touched); err != nil {
		s.markDegradedLocked(err)
		s.mu.Unlock()
		return fmt.Errorf("recount sources: %w", err)
	}
	s.clearDegradedLocked()
	snapshots := s.snapshotsLocked(touched)
	s.mu.Unlock()

	for _, snapshot := range snapshots {
		emit(eventUpdated, snapshot)
	}
	return nil
}

func (s *Service) AcknowledgeNew(sourceID string) error {
	s.mu.Lock()
	touched := map[string]bool{}
	before := map[string][]Release{}
	for id, list := range s.releases {
		if sourceID != "" && id != sourceID {
			continue
		}
		for _, r := range list {
			if r.New {
				if !touched[id] {
					before[id] = snapshotReleases(list)
				}
				r.New = false
				touched[id] = true
			}
		}
	}
	if err := s.persistTouchedLocked(touched, before); err != nil {
		s.markDegradedLocked(err)
		s.mu.Unlock()
		return fmt.Errorf("save acknowledged releases: %w", err)
	}
	s.clearDegradedLocked()
	snapshots := s.snapshotsLocked(touched)
	s.mu.Unlock()

	for _, snapshot := range snapshots {
		emit(eventUpdated, snapshot)
	}
	return nil
}

func (s *Service) PrepareDownload(releaseID string) (DownloadRequest, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r := s.findReleaseLocked(releaseID)
	if r == nil {
		return DownloadRequest{}, errReleaseNotFound
	}
	if len(r.URIs) == 0 {
		return DownloadRequest{}, errNoURI
	}
	request := DownloadRequest{
		URI:       r.URIs[0],
		Name:      r.RawTitle,
		ReleaseID: r.ID,
		SourceID:  r.SourceID,
		Version:   releaseVersion(r),
	}
	if r.CanonicalGameID != nil {
		request.GameID = *r.CanonicalGameID
	}
	return request, nil
}

func releaseVersion(r *Release) string {
	if r.ToVersion != "" {
		return r.ToVersion
	}
	return r.Version
}

//wails:ignore
func (s *Service) ReleasesFor(canonicalGameID, title string) []Release {
	if canonicalGameID == "" && title != "" && s.catalog != nil {
		if game, ok := s.catalog.LookupByTitle(title); ok {
			canonicalGameID = game.ID
		}
	}
	if canonicalGameID == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []Release
	for _, list := range s.releases {
		for _, r := range list {
			if r.Ignored || r.CanonicalGameID == nil || *r.CanonicalGameID != canonicalGameID {
				continue
			}
			out = append(out, *r)
		}
	}
	sort.Slice(out, func(a, b int) bool { return out[a].ID < out[b].ID })
	return out
}

//wails:ignore
func (s *Service) FindRelease(id string) (Release, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r := s.findReleaseLocked(id)
	if r == nil {
		return Release{}, false
	}
	return *r, true
}

// snapshotReleases copies the current values of list so a failed persist can
// restore them: list holds pointers that mutate in place, so the copy must
// be taken before those mutations, not derived from them afterwards.
func snapshotReleases(list []*Release) []Release {
	out := make([]Release, len(list))
	for i, r := range list {
		out[i] = *r
	}
	return out
}

// restoreReleasesLocked rewrites sourceID's releases back to before, which
// must have been captured by snapshotReleases from the same, unreordered
// list before it was mutated.
func (s *Service) restoreReleasesLocked(sourceID string, before []Release) {
	list := s.releases[sourceID]
	for i, r := range before {
		if i < len(list) {
			*list[i] = r
		}
	}
}

// persistTouchedLocked saves the release list for every touched source.
// Sources are independent files, so a failure for one does not stop the
// others from being attempted; each failing source's mutated releases are
// rolled back to before[sourceID] so its memory never runs ahead of its own
// file on disk (invariant I.4), while sources that saved successfully keep
// their change. All failures are combined with errors.Join for the caller.
func (s *Service) persistTouchedLocked(touched map[string]bool, before map[string][]Release) error {
	var errs []error
	for sourceID := range touched {
		if err := s.store.saveReleases(sourceID, s.releases[sourceID]); err != nil {
			s.restoreReleasesLocked(sourceID, before[sourceID])
			errs = append(errs, fmt.Errorf("save releases for source %s: %w", sourceID, err))
		}
	}
	return errors.Join(errs...)
}

// recountLocked recomputes the cached Matched/Review/Unmatched counters on
// every touched source and persists sources.json. A persist failure rolls
// those counters back to their previous values instead of leaving sources.json
// out of sync with what is now in memory (invariant I.4).
func (s *Service) recountLocked(touched map[string]bool) error {
	before := map[string]Source{}
	changed := false
	for sourceID := range touched {
		src := s.findLocked(sourceID)
		if src == nil {
			continue
		}
		before[sourceID] = *src
		matched, review, unmatched := counts(s.releases[sourceID])
		src.Matched = matched
		src.Review = review
		src.Unmatched = unmatched
		changed = true
	}
	if !changed {
		return nil
	}
	if err := s.store.saveSources(flatten(s.sources)); err != nil {
		for sourceID, snap := range before {
			if src := s.findLocked(sourceID); src != nil {
				*src = snap
			}
		}
		return fmt.Errorf("save sources: %w", err)
	}
	return nil
}

func (s *Service) snapshotsLocked(touched map[string]bool) []Source {
	out := make([]Source, 0, len(touched))
	for sourceID := range touched {
		if src := s.findLocked(sourceID); src != nil {
			out = append(out, *src)
		}
	}
	return out
}

type versionRef struct {
	raw string
	at  *time.Time
	id  string
}

type releaseAgg struct {
	count    int
	sources  map[string]bool
	title    string
	titleID  string
	versions []versionRef
}

//wails:ignore
func (s *Service) SearchReleaseMatches(query string, gameIDs []string, unmatchedLimit int) ReleaseMatches {
	search := strings.ToLower(strings.TrimSpace(query))
	matches := ReleaseMatches{Games: map[string]GameReleaseInfo{}, Unmatched: []ReleaseView{}}
	if search == "" {
		return matches
	}
	if unmatchedLimit < 0 {
		unmatchedLimit = 0
	}
	normalized := titles.Normalize(search)
	wanted := make(map[string]bool, len(gameIDs))
	for _, id := range gameIDs {
		if id != "" {
			wanted[id] = true
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	agg := map[string]*releaseAgg{}
	var unmatched []*Release
	for sourceID, list := range s.releases {
		src := s.findLocked(sourceID)
		if src == nil || !src.Enabled {
			continue
		}
		for _, r := range list {
			if r.Ignored || r.Availability != AvailabilityAvailable {
				continue
			}
			hit := releaseMatchesQuery(r, search, normalized)
			if r.CanonicalGameID == nil || *r.CanonicalGameID == "" {
				if hit {
					unmatched = append(unmatched, r)
				}
				continue
			}
			gameID := *r.CanonicalGameID
			if !hit && !wanted[gameID] {
				continue
			}
			a := agg[gameID]
			if a == nil {
				a = &releaseAgg{sources: map[string]bool{}}
				agg[gameID] = a
			}
			a.count++
			a.sources[r.SourceID] = true
			if a.titleID == "" || r.ID < a.titleID {
				a.title, a.titleID = r.Title, r.ID
			}
			a.versions = append(a.versions, versionRef{raw: releaseVersion(r), at: r.UploadedAt, id: r.ID})
		}
	}
	for gameID, a := range agg {
		matches.Games[gameID] = GameReleaseInfo{
			Title:         a.title,
			Releases:      a.count,
			Sources:       len(a.sources),
			LatestVersion: latestVersion(a.versions),
		}
	}

	sortReleases(unmatched, "")
	if len(unmatched) > unmatchedLimit {
		matches.MoreUnmatched = len(unmatched) - unmatchedLimit
		unmatched = unmatched[:unmatchedLimit]
	}
	for _, r := range unmatched {
		matches.Unmatched = append(matches.Unmatched, s.viewLocked(r))
	}
	return matches
}

func releaseMatchesQuery(r *Release, search, normalized string) bool {
	if strings.Contains(strings.ToLower(r.RawTitle), search) {
		return true
	}
	return normalized != "" && strings.Contains(r.NormalizedTitle, normalized)
}

func latestVersion(refs []versionRef) string {
	sort.Slice(refs, func(a, b int) bool {
		left, right := refs[a], refs[b]
		switch {
		case left.at == nil && right.at == nil:
		case left.at == nil:
			return false
		case right.at == nil:
			return true
		case !left.at.Equal(*right.at):
			return left.at.After(*right.at)
		}
		return left.id < right.id
	})
	var best version.Version
	raw := ""
	for _, ref := range refs {
		if ref.raw == "" {
			continue
		}
		parsed := version.Parse(ref.raw)
		if raw == "" {
			best, raw = parsed, ref.raw
			continue
		}
		if cmp, ok := version.Compare(parsed, best); ok && cmp > 0 {
			best, raw = parsed, ref.raw
		}
	}
	return raw
}

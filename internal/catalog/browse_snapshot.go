package catalog

import (
	"strings"
	"time"
	"typhon/internal/uierr"
)

type browseSnapshot struct {
	query GameQuery
	used  time.Time
}

// A continuation must reuse both the provider revision and the personal
// signals of its first page. Play sessions and favorites may change while a
// user reads a page; applying them mid-stream would duplicate or skip games.
func (s *Service) prepareBrowseSnapshot(q GameQuery) (GameQuery, error) {
	if q.Page > 1 && q.Snapshot != "" {
		s.mu.Lock()
		snap, ok := s.browseSnapshots[q.Snapshot]
		if ok && time.Since(snap.used) < 30*time.Minute {
			snap.used = time.Now()
			s.browseSnapshots[q.Snapshot] = snap
		} else {
			ok = false
		}
		s.mu.Unlock()
		if !ok {
			return GameQuery{}, uierr.Wrap("catalog.changed", ErrCatalogChanged)
		}
		frozen := snap.query
		if q.PageSize > 0 && frozen.PageSize > 0 && q.PageSize != frozen.PageSize {
			return GameQuery{}, uierr.Wrap("catalog.changed", ErrCatalogChanged)
		}
		frozen.Page, frozen.Revision, frozen.Snapshot = q.Page, q.Revision, q.Snapshot
		return frozen, nil
	}
	if q.Page > 1 && browseQueryNeedsSnapshot(q) {
		return GameQuery{}, uierr.Wrap("catalog.changed", ErrCatalogChanged)
	}
	if q.Sort == "newest" {
		q.Sort = "year"
	}
	games, p, source := s.recommendationSnapshot()
	items := []RecommendationLibraryItem(nil)
	profile := profileFromEvidence(recommendationEvidence{}, p)
	needProfile := q.Sort == "auto" || (q.Sort == "for-you" && strings.TrimSpace(q.Profile) == "")
	if needProfile || q.HideLibrary {
		if source != nil {
			items = source()
		}
	}
	if needProfile {
		profile = profileFromEvidence(buildEvidence(games, filterEvidenceItems(items, p)), p)
		if s.recommendationLoadErr != nil {
			profile = profileFromEvidence(recommendationEvidence{}, p)
		}
	}
	if q.Sort == "auto" {
		q.Sort = "popular"
		// Weak evidence contributes gradually before the UI switches its default
		// label to For you. Explicit Popular always remains non-personalized.
		if profile.Confidence > 0 {
			q.Sort = "for-you"
		}
	}
	q = s.enrichRecommendationQueryWithSnapshot(q, p, source, items, profile)
	return s.freezeBrowseQuery(q), nil
}

func (s *Service) freezeBrowseQuery(q GameQuery) GameQuery {
	if q.Page <= 1 {
		q.Snapshot = NewID()
		s.mu.Lock()
		if s.browseSnapshots == nil {
			s.browseSnapshots = make(map[string]browseSnapshot)
		}
		if len(s.browseSnapshots) >= 32 {
			oldestID := ""
			var oldest time.Time
			for id, snap := range s.browseSnapshots {
				if oldestID == "" || snap.used.Before(oldest) {
					oldestID, oldest = id, snap.used
				}
			}
			delete(s.browseSnapshots, oldestID)
		}
		frozen := q
		frozen.ExcludeIDs = append([]string(nil), q.ExcludeIDs...)
		s.browseSnapshots[q.Snapshot] = browseSnapshot{query: frozen, used: time.Now()}
		s.mu.Unlock()
	}
	return q
}

func browseQueryNeedsSnapshot(q GameQuery) bool {
	return q.Sort == "auto" || q.Sort == "for-you" || q.HideLibrary || q.HideNotInterested || strings.TrimSpace(q.Profile) != ""
}

package sources

import (
	"context"
	"fmt"

	"typhon/internal/catalog"
)

type RelatedReleases struct {
	Game   catalog.RelatedGame `json:"game"`
	GameID string              `json:"gameId,omitempty"`
	OnPC   bool                `json:"onPc"`
	Groups []ReleaseGroup      `json:"groups"`
}

// GetRelatedReleases lists the releases of games the server ties to gameID
// (remake, remaster, port...). Releases that already belong to gameID itself
// are left out: those are what GetReleasesForGame returns.
func (s *Service) GetRelatedReleases(ctx context.Context, gameID string) ([]RelatedReleases, error) {
	if gameID == "" {
		return []RelatedReleases{}, nil
	}
	if s.catalog == nil {
		return nil, errNoCatalog
	}
	related, err := s.catalog.Related(ctx, gameID)
	if err != nil {
		return nil, fmt.Errorf("load related games: %w", err)
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	pool, names := s.relatedPool()
	out := make([]RelatedReleases, 0, len(related))
	for _, rel := range related {
		if err = ctx.Err(); err != nil {
			return nil, err
		}
		item := RelatedReleases{Game: rel, OnPC: catalog.OnPC(rel.Platforms), Groups: []ReleaseGroup{}}
		localID, ok := s.catalog.LocalIDByServerID(rel.ID)
		if ok && s.catalog.SameGame(localID, gameID) {
			continue
		}
		if ok {
			item.GameID = localID
			item.Groups = s.groupsForRelated(pool, names, localID, gameID)
		}
		out = append(out, item)
	}
	return out, nil
}

// relatedPool copies the releases that can belong to a related game, so the
// catalog lookups that follow run without s.mu.
func (s *Service) relatedPool() ([]*Release, map[string]string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var pool []*Release
	names := map[string]string{}
	for _, releases := range s.releases {
		for _, r := range releases {
			if r.Ignored || r.CanonicalGameID == nil {
				continue
			}
			cp := *r
			pool = append(pool, &cp)
			if _, seen := names[cp.SourceID]; !seen {
				names[cp.SourceID] = s.sourceNameLocked(cp.SourceID)
			}
		}
	}
	return pool, names
}

func (s *Service) groupsForRelated(pool []*Release, names map[string]string, relatedID, gameID string) []ReleaseGroup {
	var list []*Release
	for _, r := range pool {
		if !s.sameGame(*r.CanonicalGameID, relatedID) || s.sameGame(*r.CanonicalGameID, gameID) {
			continue
		}
		list = append(list, r)
	}
	sortReleases(list, "")
	return groupReleases(list, func(id string) string { return names[id] })
}

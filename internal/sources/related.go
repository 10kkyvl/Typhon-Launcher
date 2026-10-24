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
	out := make([]RelatedReleases, 0, len(related))
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, rel := range related {
		item := RelatedReleases{Game: rel, OnPC: catalog.OnPC(rel.Platforms), Groups: []ReleaseGroup{}}
		localID, ok := s.catalog.LocalIDByServerID(rel.ID)
		if ok && s.catalog.SameGame(localID, gameID) {
			continue
		}
		if ok {
			item.GameID = localID
			item.Groups = s.groupsForRelatedLocked(localID, gameID)
		}
		out = append(out, item)
	}
	return out, nil
}

func (s *Service) groupsForRelatedLocked(relatedID, gameID string) []ReleaseGroup {
	var list []*Release
	for _, releases := range s.releases {
		for _, r := range releases {
			if r.Ignored || r.CanonicalGameID == nil || !s.sameGame(*r.CanonicalGameID, relatedID) || s.sameGame(*r.CanonicalGameID, gameID) {
				continue
			}
			list = append(list, r)
		}
	}
	sortReleases(list, "")
	return s.groupLocked(list)
}

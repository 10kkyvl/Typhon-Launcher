package catalog

import (
	"context"
	"errors"
)

type ReleaseQuery struct {
	Titles []string `json:"titles"`
	Year   int      `json:"year,omitempty"`
}
type ReleaseMatch struct {
	Game *Game `json:"game,omitempty"`
}
type ReleaseMatcher interface {
	MatchReleases(context.Context, []ReleaseQuery) ([]ReleaseMatch, error)
}

const MethodServerTitle Method = "server_title"

//wails:ignore
func (s *Service) HasReleaseMatcher() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.remote.(ReleaseMatcher)
	return ok
}

// ResolveSourceQueries performs network work outside the catalog/source locks
// and persists official identities before any release can reference them.
//
//wails:ignore
func (s *Service) ResolveSourceQueries(ctx context.Context, queries []ReleaseQuery) ([]Match, error) {
	resolved, err := s.PreviewSourceQueries(ctx, queries)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	previous := append([]Game(nil), s.games...)
	byServer, byIGDB, bySteam := remoteIndexes(s.games)
	out := make([]Match, len(queries))
	changed := false
	for i, r := range resolved {
		out[i] = Match{Status: StatusUnmatched, Method: MethodNone}
		if r.Game == nil {
			continue
		}
		g := *r.Game
		if g.ID == "" || g.Title == "" || g.ExternalIDs.empty() {
			s.games = previous
			return nil, errors.New("invalid catalog release identity")
		}
		g.ServerID = g.ID
		g.LocalExternalIDs = ExternalIDs{}
		g.Provisional = false
		if pos := remoteMatchPosition(g, byServer, byIGDB, bySteam); pos >= 0 {
			g = s.games[pos]
		} else {
			s.games = append(s.games, g)
			indexRemoteGame(g, len(s.games)-1, byServer, byIGDB, bySteam)
			changed = true
		}
		out[i] = single(g, scoreExactTitle, MethodServerTitle)
	}
	if changed {
		if err = s.persistGamesLocked(); err != nil {
			s.games = previous
			return nil, err
		}
		s.rebuildLocked()
	}
	return out, nil
}

// PreviewSourceQueries checks coverage without adding games to the local store.
//
//wails:ignore
func (s *Service) PreviewSourceQueries(ctx context.Context, queries []ReleaseQuery) ([]ReleaseMatch, error) {
	if len(queries) == 0 {
		return []ReleaseMatch{}, nil
	}
	s.mu.RLock()
	remote, ok := s.remote.(ReleaseMatcher)
	s.mu.RUnlock()
	if !ok {
		return nil, errors.New("catalog release matcher unavailable")
	}
	resolved, err := remote.MatchReleases(ctx, queries)
	if err != nil {
		return nil, err
	}
	if len(resolved) != len(queries) {
		return nil, errors.New("incomplete catalog release match response")
	}
	return resolved, nil
}

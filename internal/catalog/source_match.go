package catalog

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode"
)

type ReleaseQuery struct {
	Titles []string `json:"titles"`
	Year   int      `json:"year,omitempty"`
}
type ReleaseMatch struct {
	Game       *Game              `json:"game,omitempty"`
	Reason     string             `json:"reason,omitempty"`
	Candidates []ReleaseCandidate `json:"candidates,omitempty"`
}

type ReleaseCandidate struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	ReleaseYear int      `json:"releaseYear,omitempty"`
	Platforms   []string `json:"platforms,omitempty"`
	GameType    string   `json:"gameType,omitempty"`
	ParentID    string   `json:"parentId,omitempty"`
}
type ReleaseMatcher interface {
	MatchReleases(context.Context, []ReleaseQuery) ([]ReleaseMatch, error)
}

// RulesVersioner is implemented by a matcher whose server numbers its matching
// rules; a saved match from an older number is worth asking again.
type RulesVersioner interface {
	MatchRulesVersion() int
}

// Failed positions in the results are zero values, not misses.
type PartialMatchError struct {
	Failed []int
	Total  int
	Err    error
}

func (e *PartialMatchError) Error() string {
	return fmt.Sprintf("%d of %d release queries failed: %v", len(e.Failed), e.Total, e.Err)
}

func (e *PartialMatchError) Unwrap() error { return e.Err }

const MethodServerTitle Method = "server_title"

const (
	ReasonNone      = "none"
	ReasonAmbiguous = "ambiguous"
	ReasonTooMany   = "too_many"
)

const (
	SourceServer = "server"
	SourceLocal  = "local"
)

const (
	maxServerCandidates = 3
	maxCandidateID      = 128
	maxCandidateTitle   = 200
	maxCandidatePlats   = 8
	maxCandidatePlat    = 32
	minCandidateYear    = 1950
	maxCandidateYear    = 2100
)

// An answer from a server that does not know the field, or from a newer one
// with a reason this launcher has no word for, carries no reason.
func normalizeReason(reason string) string {
	switch reason {
	case ReasonNone, ReasonAmbiguous, ReasonTooMany:
		return reason
	}
	return ""
}

func cleanText(s string, limit int) string {
	s = strings.TrimSpace(strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s))
	if runes := []rune(s); len(runes) > limit {
		s = strings.TrimSpace(string(runes[:limit]))
	}
	return s
}

// serverCandidates keeps at most three well-formed candidates: a name from the
// network is shown to the user, so it is cleaned and cut before it is stored.
func serverCandidates(in []ReleaseCandidate) []Candidate {
	out := make([]Candidate, 0, maxServerCandidates)
	seen := map[string]bool{}
	for _, c := range in {
		if len(out) == maxServerCandidates {
			break
		}
		id := strings.TrimSpace(c.ID)
		title := cleanText(c.Title, maxCandidateTitle)
		if id == "" || len(id) > maxCandidateID || title == "" || seen[id] {
			continue
		}
		seen[id] = true
		cand := Candidate{GameID: id, Title: title, Method: MethodNone, Source: SourceServer}
		if c.ReleaseYear >= minCandidateYear && c.ReleaseYear <= maxCandidateYear {
			year := c.ReleaseYear
			cand.Year = &year
		}
		for _, p := range c.Platforms {
			if len(cand.Platforms) == maxCandidatePlats {
				break
			}
			if p = cleanText(p, maxCandidatePlat); p != "" {
				cand.Platforms = append(cand.Platforms, p)
			}
		}
		out = append(out, cand)
	}
	return out
}

//wails:ignore
func (s *Service) HasReleaseMatcher() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.remote.(ReleaseMatcher)
	return ok
}

//wails:ignore
func (s *Service) MatchRulesVersion() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if v, ok := s.remote.(RulesVersioner); ok {
		return v.MatchRulesVersion()
	}
	return 0
}

//wails:ignore
func (s *Service) HasGame(id string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.idx.game(id)
	return ok
}

// ResolveSourceQueries performs network work outside the catalog/source locks
// and persists official identities before any release can reference them.
//
//wails:ignore
func (s *Service) ResolveSourceQueries(ctx context.Context, queries []ReleaseQuery) ([]Match, error) {
	resolved, err := s.PreviewSourceQueries(ctx, queries)
	var partial *PartialMatchError
	switch {
	case err == nil:
	case errors.As(err, &partial):
	default:
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	previous := append([]Game(nil), s.games...)
	out := make([]Match, len(queries))
	changed := false
	for i, r := range resolved {
		out[i] = Match{Status: StatusUnmatched, Method: MethodNone}
		if r.Game == nil {
			out[i].Reason = normalizeReason(r.Reason)
			if out[i].Reason == ReasonAmbiguous {
				if list := serverCandidates(r.Candidates); len(list) > 0 {
					out[i].Status = StatusReview
					out[i].Candidates = list
				}
			}
			continue
		}
		g := *r.Game
		if g.ID == "" || g.Title == "" || g.ExternalIDs.empty() {
			s.games = previous
			if changed {
				s.rebuildLocked()
			}
			return nil, errors.New("invalid catalog release identity")
		}
		g.ServerID = g.ID
		g.LocalExternalIDs = ExternalIDs{}
		g.Provisional = false
		if pos := s.idx.matchRemote(g); pos >= 0 {
			old := s.games[pos]
			// A match answer carries a slim projection without cover, genres or
			// summary, so only the links the server owns are taken from it.
			merged := old
			if g.ParentID != "" {
				merged.ParentID = g.ParentID
			}
			if g.GameType != "" {
				merged.GameType = g.GameType
			}
			if merged.ParentID != old.ParentID || merged.GameType != old.GameType {
				s.games[pos] = merged
				s.updateIndexAtLocked(pos, old)
				changed = true
			}
			g = merged
		} else {
			s.games = append(s.games, g)
			s.addToIndexLocked(g)
			changed = true
		}
		out[i] = single(g, scoreExactTitle, MethodServerTitle)
	}
	if changed {
		if persistErr := s.persistGamesLocked(); persistErr != nil {
			s.games = previous
			s.rebuildLocked()
			return nil, persistErr
		}
		s.rebuildLocked()
	}
	return out, err
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
	var partial *PartialMatchError
	switch {
	case err == nil:
	case errors.As(err, &partial):
	default:
		return nil, err
	}
	if len(resolved) != len(queries) {
		return nil, errors.New("incomplete catalog release match response")
	}
	return resolved, err
}

package catalog

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"time"
)

const relatedCacheTTL = 10 * time.Minute

var (
	ErrRelatedNotFound = errors.New("game is not in the server catalog")
	errNoRemote        = errors.New("catalog backend unavailable")
)

var relationKinds = []string{"original", "base", "remake", "remaster", "expanded", "port", "edition", "update"}

// RelatedCatalog is the part of the server client that knows related games.
// It is separate from RemoteCatalog so fixtures of the browse path stay small.
type RelatedCatalog interface {
	Related(ctx context.Context, serverID string) ([]RelatedGame, error)
}

type RelatedGame struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	GameType    string   `json:"gameType,omitempty"`
	Relation    string   `json:"relation"`
	ReleaseYear *int     `json:"releaseYear,omitempty"`
	Platforms   []string `json:"platforms,omitempty"`
	CoverURL    string   `json:"coverUrl,omitempty"`
}

// ValidRelation reports whether the server named a relation this launcher knows.
func ValidRelation(relation string) bool {
	return slices.Contains(relationKinds, relation)
}

// OnPC reports whether a game can run on a PC. An unknown platform list counts
// as PC: hiding a game because the server did not say where it runs is worse
// than showing it.
func OnPC(platforms []string) bool {
	if len(platforms) == 0 {
		return true
	}
	for _, p := range platforms {
		if strings.EqualFold(p, "PC (Microsoft Windows)") || strings.EqualFold(p, "DOS") {
			return true
		}
	}
	return false
}

type relatedEntry struct {
	items []RelatedGame
	at    time.Time
}

type relatedCache struct {
	mu      sync.Mutex
	entries map[string]relatedEntry
}

func (c *relatedCache) get(id string, now time.Time) ([]RelatedGame, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[id]
	if !ok || now.Sub(e.at) >= relatedCacheTTL {
		return nil, false
	}
	return slices.Clone(e.items), true
}

func (c *relatedCache) reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = nil
}

func (c *relatedCache) put(id string, items []RelatedGame, now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.entries == nil {
		c.entries = map[string]relatedEntry{}
	}
	for key, e := range c.entries {
		if now.Sub(e.at) >= relatedCacheTTL {
			delete(c.entries, key)
		}
	}
	c.entries[id] = relatedEntry{items: slices.Clone(items), at: now}
}

// Related lists the server's records tied to a game by remake, remaster, port
// and the like. It does not change what counts as the same game.
//
//wails:ignore
func (s *Service) Related(ctx context.Context, gameID string) ([]RelatedGame, error) {
	s.mu.RLock()
	remote := s.remote
	game, ok := s.idx.game(s.resolveIDLocked(gameID))
	if !ok {
		game, ok = s.discoveryGames[gameID].game, s.discoveryGames[gameID].game.ID != ""
	}
	s.mu.RUnlock()
	source, capable := remote.(RelatedCatalog)
	if !capable {
		return nil, errNoRemote
	}
	if !ok || game.ServerID == "" {
		return []RelatedGame{}, nil
	}
	now := s.clock()
	if items, hit := s.related.get(game.ServerID, now); hit {
		return items, nil
	}
	items, err := source.Related(ctx, game.ServerID)
	if errors.Is(err, ErrRelatedNotFound) {
		items, err = []RelatedGame{}, nil
	}
	if err != nil {
		return nil, err
	}
	if items == nil {
		items = []RelatedGame{}
	}
	s.related.put(game.ServerID, items, now)
	return slices.Clone(items), nil
}

// LocalIDByServerID returns the saved record for a server catalog id.
//
//wails:ignore
func (s *Service) LocalIDByServerID(serverID string) (string, bool) {
	if serverID == "" {
		return "", false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	list := s.idx.byServer[serverID]
	if len(list) == 0 {
		return "", false
	}
	return s.games[list[len(list)-1]].ID, true
}

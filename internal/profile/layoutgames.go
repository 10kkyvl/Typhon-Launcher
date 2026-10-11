package profile

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"typhon/internal/account"
	"typhon/internal/library"
)

const (
	blockPinned     = "pinned"
	blockCollection = "collection"
)

const ErrUnresolved = "unresolved"

type LayoutGame struct {
	Game  *GameRef `json:"game,omitempty"`
	Error string   `json:"error,omitempty"`
}

func layoutIGDBIDs(blocks []account.LayoutBlock) ([]int64, error) {
	var ids []int64
	seen := map[int64]struct{}{}
	add := func(blockID string, id int64) error {
		if id <= 0 {
			return fmt.Errorf("layout block %q: invalid igdb id %d", blockID, id)
		}
		if _, dup := seen[id]; dup {
			return nil
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
		return nil
	}
	for _, block := range blocks {
		switch block.Type {
		case blockPinned:
			var config struct {
				IGDBID int64 `json:"igdbId"`
			}
			if err := json.Unmarshal(block.Config, &config); err != nil {
				return nil, fmt.Errorf("layout block %q: decode pinned config: %w", block.ID, err)
			}
			if err := add(block.ID, config.IGDBID); err != nil {
				return nil, err
			}
		case blockCollection:
			var config struct {
				IGDBIDs []int64 `json:"igdbIds"`
			}
			if err := json.Unmarshal(block.Config, &config); err != nil {
				return nil, fmt.Errorf("layout block %q: decode collection config: %w", block.ID, err)
			}
			for _, id := range config.IGDBIDs {
				if err := add(block.ID, id); err != nil {
					return nil, err
				}
			}
		}
	}
	return ids, nil
}

func (s *Service) layoutGames(blocks []account.LayoutBlock, games []library.Game) (map[string]LayoutGame, error) {
	ids, err := layoutIGDBIDs(blocks)
	if err != nil {
		return nil, err
	}
	resolved := make(map[string]LayoutGame, len(ids))
	if len(ids) == 0 {
		return resolved, nil
	}

	owned := map[string]library.Game{}
	for _, g := range games {
		if g.CanonicalGameID == "" {
			continue
		}
		igdbID := s.catalog.IGDBIDOf(g.CanonicalGameID)
		if igdbID == "" {
			continue
		}
		if known, ok := owned[igdbID]; ok && !known.Archived {
			continue
		}
		owned[igdbID] = g
	}

	for _, id := range ids {
		key := strconv.FormatInt(id, 10)
		known, ok := s.catalog.GameByIGDB(key)
		if !ok {
			resolved[key] = LayoutGame{Error: ErrUnresolved}
			continue
		}
		if g, ok := owned[key]; ok {
			r := ref(g)
			resolved[key] = LayoutGame{Game: &r}
			continue
		}
		resolved[key] = LayoutGame{Game: &GameRef{ID: known.ID, Title: known.Title, Cover: known.CoverURL, CanonicalGameID: known.ID}}
	}
	return resolved, nil
}

func (s *Service) IGDBIDs(ctx context.Context, canonicalGameIDs []string) (map[string]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	out := make(map[string]string, len(canonicalGameIDs))
	for i, id := range canonicalGameIDs {
		if i%256 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		if id == "" {
			continue
		}
		if igdbID := s.catalog.IGDBIDOf(id); igdbID != "" {
			out[id] = igdbID
		}
	}
	return out, nil
}

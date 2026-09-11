package catalog

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"path/filepath"
	"time"

	"typhon/internal/storage"
	"typhon/internal/uierr"
)

var ErrCatalogChanged = errors.New("catalog changed; reload from the first page")

type RemoteCatalog interface {
	Browse(context.Context, GameQuery) (GamePage, error)
}

//wails:ignore
func (s *Service) SetRemoteCatalog(remote RemoteCatalog) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.remote = remote
}

// BrowseGames is the public catalog. Local games are a personal/cache store,
// never the membership source for a successful online response.
func (s *Service) BrowseGames(q GameQuery) (GamePage, error) {
	s.mu.RLock()
	remote := s.remote
	dir := filepath.Dir(s.gamesPath)
	s.mu.RUnlock()
	raw, _ := json.Marshal(q)
	sum := sha256.Sum256(raw)
	path := filepath.Join(dir, "catalog-pages", hex.EncodeToString(sum[:])+".json")
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	var page GamePage
	var err error
	if remote == nil {
		err = errors.New("catalog backend unavailable")
	} else {
		page, err = remote.Browse(ctx, q)
	}
	if err != nil {
		if errors.Is(err, ErrCatalogChanged) {
			return GamePage{}, uierr.Wrap("catalog.changed", err)
		}
		if loadErr := storage.Load(path, 1, nil, &page); loadErr != nil {
			return GamePage{}, err
		}
		page.Offline = true
		return page, nil
	}
	s.mu.Lock()
	previous := append([]Game(nil), s.games...)
	for i, g := range page.Items {
		// Existing personal references retain their IDs. ServerID records the
		// provider-independent identity without rewriting installation provenance.
		g.ServerID = g.ID
		s.reconcileRemoteLinksLocked(g)
		for _, old := range s.games {
			if old.ServerID == g.ServerID || (g.ExternalIDs.IGDB != "" && old.ExternalIDs.IGDB == g.ExternalIDs.IGDB) || (g.ExternalIDs.IGDB == "" && old.ExternalIDs.IGDB == "" && g.ExternalIDs.Steam != "" && old.ExternalIDs.Steam == g.ExternalIDs.Steam) {
				g.ID = old.ID
				break
			}
		}
		found := false
		for j, old := range s.games {
			if old.ID == g.ID {
				if old.MetadataUpdatedAt != nil {
					g.Summary = old.Summary
					g.Developer = old.Developer
					g.Publisher = old.Publisher
					g.CoverAssetID = old.CoverAssetID
					g.HeroAssetID = old.HeroAssetID
					g.MetadataUpdatedAt = old.MetadataUpdatedAt
					g.MetadataLanguage = old.MetadataLanguage
				}
				s.games[j] = g
				found = true
				break
			}
		}
		if !found {
			s.games = append(s.games, g)
		}
		if c, ok := page.Compat[g.ServerID]; ok && g.ID != g.ServerID {
			delete(page.Compat, g.ServerID)
			page.Compat[g.ID] = c
		}
		page.Items[i] = g
	}
	s.rebuildLocked()
	for i := range page.Items {
		for _, old := range s.games {
			if old.ID != page.Items[i].ID && s.sameGameLocked(old.ID, page.Items[i].ID) {
				page.Items[i].AliasIDs = append(page.Items[i].AliasIDs, old.ID)
			}
		}
	}
	if err = s.persistGamesLocked(); err != nil {
		s.games = previous
		s.rebuildLocked()
		s.mu.Unlock()
		return GamePage{}, err
	}
	s.mu.Unlock()
	page.CachedAt = time.Now().UTC()
	if err = storage.Save(path, 1, page); err != nil {
		return GamePage{}, err
	}
	return page, nil
}

//wails:ignore
func (s *Service) HasRemoteCatalog() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.remote != nil
}

// A new authoritative claim invalidates stale claims on other visited games,
// even when those other games are outside the current result page.
func (s *Service) reconcileRemoteLinksLocked(current Game) {
	claimed := map[string]bool{}
	igdbClaims := map[string]bool{}
	for _, id := range current.ProviderLinks["igdb"] {
		igdbClaims[id] = true
	}
	if current.ExternalIDs.IGDB != "" {
		igdbClaims[current.ExternalIDs.IGDB] = true
	}
	for _, id := range current.ProviderLinks["steam"] {
		claimed[id] = true
	}
	if current.ExternalIDs.Steam != "" {
		claimed[current.ExternalIDs.Steam] = true
	}
	for i, old := range s.games {
		if old.ServerID == current.ServerID || old.ExternalIDs.IGDB == "" || (current.ExternalIDs.IGDB != "" && old.ExternalIDs.IGDB == current.ExternalIDs.IGDB) {
			continue
		}
		kept := []string{}
		changed := false
		for _, id := range old.ProviderLinks["steam"] {
			if claimed[id] {
				changed = true
			} else {
				kept = append(kept, id)
			}
		}
		if claimed[old.ExternalIDs.Steam] {
			old.ExternalIDs.Steam = ""
			changed = true
		}
		keptIGDB := []string{}
		for _, id := range old.ProviderLinks["igdb"] {
			if igdbClaims[id] {
				changed = true
			} else {
				keptIGDB = append(keptIGDB, id)
			}
		}
		if !changed {
			continue
		}
		links := map[string][]string{}
		for provider, ids := range old.ProviderLinks {
			links[provider] = ids
		}
		links["steam"] = kept
		links["igdb"] = keptIGDB
		old.ProviderLinks = links
		s.games[i] = old
	}
}

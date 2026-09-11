package catalog

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"

	"typhon/internal/storage"
	"typhon/internal/uierr"
)

var ErrCatalogChanged = errors.New("catalog changed; reload from the first page")

const (
	maxCatalogPageCacheEntries = 128
	catalogPageCacheMaxAge     = 30 * 24 * time.Hour
)

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
	raw, marshalErr := json.Marshal(q)
	if marshalErr != nil {
		return GamePage{}, marshalErr
	}
	sum := sha256.Sum256(raw)
	path := filepath.Join(dir, "catalog-pages", hex.EncodeToString(sum[:])+".json")
	//nolint:forbidigo // Wails RPC entry point: this service has no lifecycle context; each request is bounded and canceled on return.
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
		page = GamePage{}
		if loadErr := storage.Load(path, 1, nil, &page); loadErr != nil {
			return GamePage{}, err
		}
		page.Offline = true
		s.normalizeCachedPage(&page)
		return page, nil
	}
	s.mu.Lock()
	previous := append([]Game(nil), s.games...)
	complete := remoteProviderCompleteness(page.Providers)
	byServer, byIGDB, bySteam := remoteIndexes(s.games)
	claims := make([]Game, len(page.Items))
	for i, g := range page.Items {
		// LocalExternalIDs is a private durability detail. A remote response
		// must never be able to inject it, and it must not leak through the
		// public page returned to the frontend.
		g.LocalExternalIDs = ExternalIDs{}
		// Existing personal references retain their IDs. ServerID records the
		// provider-independent identity without rewriting installation provenance.
		g.ServerID = g.ID
		claims[i] = g
		pos := remoteMatchPosition(g, byServer, byIGDB, bySteam)
		if pos >= 0 {
			old := s.games[pos]
			g.ID = old.ID
			g = mergeRemoteGame(old, g, complete)
			unindexRemoteGame(old, pos, byServer, byIGDB, bySteam)
			s.games[pos] = g
		} else {
			s.games = append(s.games, g)
			pos = len(s.games) - 1
		}
		indexRemoteGame(g, pos, byServer, byIGDB, bySteam)
		if c, ok := page.Compat[g.ServerID]; ok && g.ID != g.ServerID {
			delete(page.Compat, g.ServerID)
			page.Compat[g.ID] = c
		}
		g.AliasIDs = nil
		g.LocalExternalIDs = ExternalIDs{}
		page.Items[i] = g
	}
	s.reconcileRemotePageLinksLocked(claims)
	changed := !reflect.DeepEqual(previous, s.games)
	if changed {
		s.rebuildLocked()
	}
	pageAliases := s.remotePageAliasesLocked(page.Items)
	for i := range page.Items {
		page.Items[i].AliasIDs = pageAliases[i]
	}
	if changed {
		if err = s.persistGamesLocked(); err != nil {
			s.games = previous
			s.rebuildLocked()
			s.mu.Unlock()
			return GamePage{}, err
		}
	}
	s.mu.Unlock()
	page.CachedAt = time.Now().UTC()
	if err = storage.Save(path, 1, page); err != nil {
		return GamePage{}, err
	}
	pruneCatalogPageCache(filepath.Dir(path), page.CachedAt)
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
func (s *Service) reconcileRemotePageLinksLocked(currentGames []Game) {
	claimed := map[string]bool{}
	igdbClaims := map[string]bool{}
	currentServers := map[string]bool{}
	currentIGDB := map[string]bool{}
	for _, current := range currentGames {
		if current.ServerID != "" {
			currentServers[current.ServerID] = true
		}
		if current.ExternalIDs.IGDB != "" {
			currentIGDB[current.ExternalIDs.IGDB] = true
		}
		if len(current.ProviderLinks["igdb"]) > 0 || current.ExternalIDs.IGDB != "" {
			for _, id := range current.ProviderLinks["igdb"] {
				igdbClaims[id] = true
			}
			if current.ExternalIDs.IGDB != "" {
				igdbClaims[current.ExternalIDs.IGDB] = true
			}
		}
		if len(current.ProviderLinks["steam"]) > 0 || current.ExternalIDs.Steam != "" {
			for _, id := range current.ProviderLinks["steam"] {
				claimed[id] = true
			}
			if current.ExternalIDs.Steam != "" {
				claimed[current.ExternalIDs.Steam] = true
			}
		}
	}
	for i, old := range s.games {
		if (old.ServerID != "" && currentServers[old.ServerID]) || old.ExternalIDs.IGDB == "" || currentIGDB[old.ExternalIDs.IGDB] {
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
		if old.LocalExternalIDs.Steam != "" && claimed[old.LocalExternalIDs.Steam] {
			old.LocalExternalIDs.Steam = ""
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
		if old.LocalExternalIDs.IGDB != "" && igdbClaims[old.LocalExternalIDs.IGDB] {
			old.LocalExternalIDs.IGDB = ""
			changed = true
		}
		if !changed {
			continue
		}
		links := cloneProviderLinks(old.ProviderLinks)
		if links == nil {
			links = make(map[string][]string)
		}
		links["steam"] = kept
		links["igdb"] = keptIGDB
		old.ProviderLinks = links
		s.games[i] = old
	}
}

func remoteProviderCompleteness(statuses []IndexStatus) map[string]bool {
	if len(statuses) == 0 {
		return nil
	}
	complete := make(map[string]bool, len(statuses))
	for _, status := range statuses {
		provider := strings.ToLower(strings.TrimSpace(status.Provider))
		if provider != "" {
			complete[provider] = status.Complete
		}
	}
	return complete
}

func remoteProviderComplete(complete map[string]bool, provider string) bool {
	if complete == nil {
		return true
	}
	value, known := complete[strings.ToLower(provider)]
	return !known || value
}

func remoteIndexes(games []Game) (byServer, byIGDB, bySteam map[string]int) {
	byServer = make(map[string]int, len(games))
	byIGDB = make(map[string]int, len(games))
	bySteam = make(map[string]int, len(games))
	for pos, game := range games {
		indexRemoteGame(game, pos, byServer, byIGDB, bySteam)
	}
	return byServer, byIGDB, bySteam
}

func remoteMatchPosition(game Game, byServer, byIGDB, bySteam map[string]int) int {
	if game.ServerID != "" {
		if pos, ok := byServer[game.ServerID]; ok {
			return pos
		}
	}
	if game.ExternalIDs.IGDB != "" {
		if pos, ok := byIGDB[strings.ToLower(game.ExternalIDs.IGDB)]; ok {
			return pos
		}
	}
	// A Steam-only server row may reuse an existing local Steam record. Once
	// the row has a confirmed IGDB id, a matching Steam id can belong to a
	// different local record and must be reconciled as a provider claim.
	if game.ExternalIDs.IGDB == "" && game.ExternalIDs.Steam != "" {
		if pos, ok := bySteam[strings.ToLower(game.ExternalIDs.Steam)]; ok {
			return pos
		}
	}
	return -1
}

func indexRemoteGame(game Game, pos int, byServer, byIGDB, bySteam map[string]int) {
	if game.ServerID != "" {
		byServer[game.ServerID] = pos
	}
	if game.ExternalIDs.IGDB != "" {
		key := strings.ToLower(game.ExternalIDs.IGDB)
		if _, exists := byIGDB[key]; !exists {
			byIGDB[key] = pos
		}
	}
	if game.ExternalIDs.Steam != "" {
		key := strings.ToLower(game.ExternalIDs.Steam)
		if _, exists := bySteam[key]; !exists {
			bySteam[key] = pos
		}
	}
}

func unindexRemoteGame(game Game, pos int, byServer, byIGDB, bySteam map[string]int) {
	if game.ServerID != "" && byServer[game.ServerID] == pos {
		delete(byServer, game.ServerID)
	}
	if game.ExternalIDs.IGDB != "" {
		key := strings.ToLower(game.ExternalIDs.IGDB)
		if byIGDB[key] == pos {
			delete(byIGDB, key)
		}
	}
	if game.ExternalIDs.Steam != "" {
		key := strings.ToLower(game.ExternalIDs.Steam)
		if bySteam[key] == pos {
			delete(bySteam, key)
		}
	}
}

func mergeRemoteGame(old, remote Game, complete map[string]bool) Game {
	merged := remote
	merged.ProviderLinks = cloneProviderLinks(remote.ProviderLinks)
	if merged.ProviderLinks == nil &&
		(!remoteProviderComplete(complete, "igdb") || !remoteProviderComplete(complete, "steam")) {
		merged.ProviderLinks = make(map[string][]string)
	}
	if !remoteProviderComplete(complete, "igdb") && remote.ExternalIDs.IGDB == "" && len(remote.ProviderLinks["igdb"]) == 0 {
		merged.ExternalIDs.IGDB = old.ExternalIDs.IGDB
		merged.ProviderLinks["igdb"] = append([]string(nil), old.ProviderLinks["igdb"]...)
	}
	if !remoteProviderComplete(complete, "steam") && remote.ExternalIDs.Steam == "" && len(remote.ProviderLinks["steam"]) == 0 {
		merged.ExternalIDs.Steam = old.ExternalIDs.Steam
		merged.ProviderLinks["steam"] = append([]string(nil), old.ProviderLinks["steam"]...)
	}
	// Personal records do not have ServerID. Their provider IDs are local
	// evidence and must survive a partial server projection.
	if old.ServerID == "" {
		if old.LocalExternalIDs.IGDB == "" {
			merged.LocalExternalIDs.IGDB = old.ExternalIDs.IGDB
		}
		if old.LocalExternalIDs.Steam == "" {
			merged.LocalExternalIDs.Steam = old.ExternalIDs.Steam
		}
		if merged.ExternalIDs.IGDB == "" {
			merged.ExternalIDs.IGDB = merged.LocalExternalIDs.IGDB
		}
		if merged.ExternalIDs.Steam == "" {
			merged.ExternalIDs.Steam = merged.LocalExternalIDs.Steam
		}
	} else {
		merged.LocalExternalIDs = old.LocalExternalIDs
		if merged.ExternalIDs.IGDB == "" {
			merged.ExternalIDs.IGDB = old.LocalExternalIDs.IGDB
		}
		if merged.ExternalIDs.Steam == "" {
			merged.ExternalIDs.Steam = old.LocalExternalIDs.Steam
		}
	}
	merged.Aliases = mergeRemoteStrings(old.Aliases, remote.Aliases)
	if old.MetadataUpdatedAt != nil {
		merged.Summary = old.Summary
		if merged.Developer == "" {
			merged.Developer = old.Developer
		}
		if merged.Publisher == "" {
			merged.Publisher = old.Publisher
		}
		merged.CoverAssetID = old.CoverAssetID
		merged.HeroAssetID = old.HeroAssetID
		merged.MetadataUpdatedAt = cloneTimePointer(old.MetadataUpdatedAt)
		merged.MetadataLanguage = old.MetadataLanguage
	}
	return merged
}

func mergeRemoteStrings(first, second []string) []string {
	if len(first) == 0 && len(second) == 0 {
		return nil
	}
	out := make([]string, 0, len(first)+len(second))
	seen := make(map[string]bool, len(first)+len(second))
	for _, values := range [][]string{first, second} {
		for _, value := range values {
			value = strings.TrimSpace(value)
			if value == "" || seen[value] {
				continue
			}
			seen[value] = true
			out = append(out, value)
		}
	}
	return out
}

func cloneProviderLinks(in map[string][]string) map[string][]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string][]string, len(in))
	for provider, ids := range in {
		out[provider] = append([]string(nil), ids...)
	}
	return out
}

func cloneTimePointer(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

// Include both providers when finding personal references to a canonical row.
// The SameGame check rejects conflicting IGDB identities sharing a stale Steam ID.
func (s *Service) remotePageAliasesLocked(items []Game) [][]string {
	byKey := map[string][]string{}
	for _, game := range s.games {
		id := s.resolveIDLocked(game.ID)
		if id == "" {
			continue
		}
		for _, key := range remoteIdentityKeys(game) {
			byKey[key] = append(byKey[key], id)
		}
	}
	aliases := make([][]string, len(items))
	for i, game := range items {
		seen := map[string]bool{}
		canonical := s.resolveIDLocked(game.ID)
		for _, key := range remoteIdentityKeys(game) {
			for _, id := range byKey[key] {
				if id != canonical && !seen[id] && s.sameGameLocked(canonical, id) {
					seen[id] = true
					aliases[i] = append(aliases[i], id)
				}
			}
		}
	}
	return aliases
}

func remoteIdentityKeys(game Game) []string {
	keys := []string{}
	for provider, id := range map[string]string{"igdb": game.ExternalIDs.IGDB, "steam": game.ExternalIDs.Steam} {
		if id != "" {
			keys = append(keys, provider+":"+strings.ToLower(id))
		}
		for _, linked := range game.ProviderLinks[provider] {
			if linked != "" {
				keys = append(keys, provider+":"+strings.ToLower(linked))
			}
		}
	}
	sort.Strings(keys)
	return keys
}

// A cached page retains membership, but old provider duplicates can be folded
// using confirmed links learned since it was saved. No personal record is added.
func (s *Service) normalizeCachedPage(page *GamePage) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	items := make([]Game, 0, len(page.Items))
	for _, cached := range page.Items {
		game := cached
		if current, ok := s.idx.game(s.resolveIDLocked(cached.ID)); ok {
			game = current
		}
		duplicate := -1
		for i, prior := range items {
			if s.sameGameLocked(prior.ID, game.ID) {
				duplicate = i
				break
			}
		}
		if duplicate < 0 {
			items = append(items, game)
			continue
		}
		// An IGDB provider home is preferred to a Steam home enriched by details.
		if len(game.ProviderLinks["igdb"]) > len(items[duplicate].ProviderLinks["igdb"]) {
			items[duplicate] = game
		}
	}
	page.Total = max(len(items), page.Total-(len(page.Items)-len(items)))
	page.Items = items
	aliases := s.remotePageAliasesLocked(items)
	for i := range page.Items {
		page.Items[i].AliasIDs = aliases[i]
		page.Items[i].LocalExternalIDs = ExternalIDs{}
	}
}

func pruneCatalogPageCache(dir string, now time.Time) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	type cachedPage struct {
		path    string
		modTime time.Time
	}
	pages := make([]cachedPage, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		if now.Sub(info.ModTime()) > catalogPageCacheMaxAge {
			if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
				slog.Debug("prune catalog page", "path", path, "error", err)
			}
			continue
		}
		pages = append(pages, cachedPage{path: path, modTime: info.ModTime()})
	}
	sort.Slice(pages, func(i, j int) bool { return pages[i].modTime.Before(pages[j].modTime) })
	for len(pages) > maxCatalogPageCacheEntries {
		if err := os.Remove(pages[0].path); err != nil && !errors.Is(err, os.ErrNotExist) {
			slog.Debug("prune catalog page", "path", pages[0].path, "error", err)
		}
		pages = pages[1:]
	}
}

package catalog

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strings"
	"time"

	"typhon/internal/storage"
	"typhon/internal/uierr"
)

var ErrBackendOutdated = uierr.New("catalog.backend_outdated", "catalog backend needs an update")

var ErrCatalogChanged = errors.New("catalog changed; reload from the first page")

const (
	maxCatalogPageCacheEntries = 128
	catalogPageCacheMaxAge     = 30 * 24 * time.Hour
	catalogPagePruneInterval   = 10 * time.Minute
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
// never the membership source for a successful online response. Wails
// supplies ctx for every bound call without changing the JS signature; a
// canceled ctx (the frontend navigated away, or the promise was aborted)
// must abort the backend request and leave no trace in catalog.json or the
// offline page cache.
func (s *Service) BrowseGames(ctx context.Context, q GameQuery) (GamePage, error) {
	return s.browseGames(ctx, q, true)
}

func (s *Service) browseGames(ctx context.Context, q GameQuery, durable bool) (GamePage, error) {
	started := time.Now()
	var prepareErr error
	q, prepareErr = s.prepareBrowseSnapshot(q)
	if prepareErr != nil {
		return GamePage{}, prepareErr
	}
	return s.browsePrepared(ctx, q, durable, time.Since(started))
}

func (s *Service) browsePreparedGames(ctx context.Context, q GameQuery, durable bool) (GamePage, error) {
	return s.browsePrepared(ctx, q, durable, 0)
}

type browseTiming struct {
	prepare time.Duration
	network time.Duration
	merge   time.Duration
	persist time.Duration
	changed bool
}

func durationMS(d time.Duration) float64 {
	return float64(d) / float64(time.Millisecond)
}

func (s *Service) browsePrepared(ctx context.Context, q GameQuery, durable bool, prepared time.Duration) (result GamePage, resultErr error) {
	entered := time.Now()
	timing := browseTiming{prepare: prepared}
	defer func() {
		if resultErr != nil {
			return
		}
		slog.Debug("catalog browse", "page", q.Page, "prepare_ms", durationMS(timing.prepare), "network_ms", durationMS(timing.network),
			"merge_ms", durationMS(timing.merge), "persist_ms", durationMS(timing.persist), "items", len(result.Items), "changed", timing.changed)
	}()
	snapshot := q.Snapshot
	q.Snapshot = ""
	allowStale := q.AllowStale
	q.AllowStale = false
	s.mu.RLock()
	remote := s.remote
	dir := filepath.Dir(s.gamesPath)
	s.mu.RUnlock()
	raw, marshalErr := json.Marshal(q)
	if marshalErr != nil {
		return GamePage{}, marshalErr
	}
	sum := sha256.Sum256(raw)
	key := hex.EncodeToString(sum[:])
	path := filepath.Join(dir, "catalog-pages", key+".json")
	if !durable {
		s.mu.RLock()
		cached, ok := s.discoveryPages[path]
		s.mu.RUnlock()
		if ok && time.Since(cached.CachedAt) < 2*time.Minute {
			cached.Snapshot = snapshot
			if cached.PersonalizationFallback {
				s.mu.Lock()
				if snap, ok := s.browseSnapshots[snapshot]; ok {
					snap.query.Sort, snap.query.Profile = "popular", ""
					s.browseSnapshots[snapshot] = snap
				}
				s.mu.Unlock()
			}
			return s.previewRemotePage(cached), nil
		}
	}
	timing.prepare += time.Since(entered)
	if stale, ok := s.staleFirstPage(q, allowStale, durable, remote, path, key, snapshot); ok { //nolint:contextcheck // invariant 19: the refresh outlives the request, so it runs under the service lifetime ctx and is tracked by wg
		return stale, nil
	}
	var page GamePage
	var err error
	if remote == nil {
		err = errors.New("catalog backend unavailable")
	} else {
		page, err = s.fetchRemotePage(ctx, q, remote, snapshot, &timing)
	}
	if err != nil {
		if errors.Is(err, ErrBackendOutdated) {
			return GamePage{}, err
		}
		if errors.Is(err, ErrCatalogChanged) {
			return GamePage{}, uierr.Wrap("catalog.changed", err)
		}
		// An explicitly canceled request must not fall back to silently serving
		// stale offline data: the caller asked to stop, not to substitute a
		// cached answer. A plain timeout (DeadlineExceeded) is unaffected and
		// still falls through to the offline cache below.
		if errors.Is(err, context.Canceled) {
			return GamePage{}, err
		}
		if !durable {
			return GamePage{}, err
		}
		page = GamePage{}
		if loadErr := storage.Load(path, 1, nil, &page); loadErr != nil {
			return GamePage{}, err
		}
		page.Offline = true
		page.Key = key
		s.normalizeCachedPage(&page)
		page.Snapshot = snapshot
		page.PersonalizationFallback = page.PersonalizationFallback || s.recommendationLoadErr != nil
		return page, nil
	}
	if !durable {
		page.CachedAt = time.Now().UTC()
		s.mu.Lock()
		if s.discoveryPages == nil {
			s.discoveryPages = map[string]GamePage{}
		}
		if len(s.discoveryPages) >= 12 {
			oldest := ""
			for key, cached := range s.discoveryPages {
				if oldest == "" || cached.CachedAt.Before(s.discoveryPages[oldest].CachedAt) {
					oldest = key
				}
			}
			delete(s.discoveryPages, oldest)
		}
		s.discoveryPages[path] = page
		s.mu.Unlock()
		page.Snapshot = snapshot
		return s.previewRemotePage(page), nil
	}
	return s.commitPage(page, path, key, snapshot, &timing)
}

// commitPage merges a freshly fetched first or later page into catalog.json,
// writes the offline page cache and returns the page as the frontend sees it.
// The synchronous browse and the background refresh share it, so both leave
// identical state behind.
func (s *Service) commitPage(page GamePage, path, key, snapshot string, timing *browseTiming) (GamePage, error) {
	var err error
	mergeStart := time.Now()
	s.mu.Lock()
	complete := remoteProviderCompleteness(page.Providers)
	claims := make([]Game, len(page.Items))
	// touched remembers, per position in s.games, the value it had before this
	// call mutated it — the first time it is mutated, never a later one. It is
	// the rollback log for persistGamesLocked failing (invariant: memory only
	// moves after persist succeeds) and it replaces copying and diffing the
	// entire multi-thousand-game slice on every browsed page: a page touches a
	// handful of existing records at most, so this stays O(page), not O(catalog).
	touched := map[int]Game{}
	changed := false
	now := time.Now()
	for i, g := range page.Items {
		// LocalExternalIDs is a private durability detail. A remote response
		// must never be able to inject it, and it must not leak through the
		// public page returned to the frontend.
		g.LocalExternalIDs = ExternalIDs{}
		// Existing personal references retain their IDs. ServerID records the
		// provider-independent identity without rewriting installation provenance.
		g.ServerID = g.ID
		claims[i] = g
		pos := s.idx.matchRemote(g)
		if pos >= 0 {
			old := s.games[pos]
			g.ID = old.ID
			merged := mergeRemoteGame(old, g, complete)
			merged.Genres = canonicalGenres(merged.Genres)
			if !reflect.DeepEqual(old, merged) {
				if _, ok := touched[pos]; !ok {
					touched[pos] = old
				}
				s.games[pos] = merged
				s.updateIndexAtLocked(pos, old)
				changed = true
			}
			g = merged
		} else {
			// A page item that matches nothing already saved is a preview, exactly
			// like a discovery candidate: shown to the user, but committed to
			// catalog.json only by GetGame (the first durable interaction). Adding
			// it here unconditionally is the bug this rewrite fixes — every page
			// scrolled past used to grow the catalog forever.
			g.Genres = canonicalGenres(g.Genres)
			s.rememberBrowsedGameLocked(g, now)
		}
		if c, ok := page.Compat[g.ServerID]; ok && g.ID != g.ServerID {
			delete(page.Compat, g.ServerID)
			page.Compat[g.ID] = c
		}
		g.AliasIDs = nil
		g.LocalExternalIDs = ExternalIDs{}
		page.Items[i] = g
	}
	if s.reconcileRemotePageLinksLocked(claims, touched) {
		changed = true
	}
	pageAliases := s.remotePageAliasesLocked(page.Items)
	for i := range page.Items {
		page.Items[i].AliasIDs = pageAliases[i]
	}
	timing.merge = time.Since(mergeStart)
	timing.changed = changed
	persistStart := time.Now()
	if changed {
		if err = s.persistGamesLocked(); err != nil {
			for pos, old := range touched {
				current := s.games[pos]
				s.games[pos] = old
				s.updateIndexAtLocked(pos, current)
			}
			s.mu.Unlock()
			return GamePage{}, err
		}
	}
	s.mu.Unlock()
	page.CachedAt = s.clock().UTC()
	if err = storage.Save(path, 1, page); err != nil {
		return GamePage{}, err
	}
	if pruneErr := s.pruneCatalogPagesIfDue(filepath.Dir(path), page.CachedAt); pruneErr != nil {
		slog.Warn("prune catalog page cache", "error", pruneErr)
	}
	timing.persist = time.Since(persistStart)
	page.Key = key
	page.Snapshot = snapshot
	page.PersonalizationFallback = page.PersonalizationFallback || s.recommendationLoadErr != nil
	return page, nil
}

// fetchRemotePage asks the backend for one page under the 25 second budget
// and, for a failed personalized first page, retries once as the general feed.
func (s *Service) fetchRemotePage(ctx context.Context, q GameQuery, remote RemoteCatalog, snapshot string, timing *browseTiming) (GamePage, error) {
	ctx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	networkStart := time.Now()
	page, err := remote.Browse(ctx, q)
	timing.network += time.Since(networkStart)
	if err != nil && remote != nil && q.Sort == "for-you" && q.Page <= 1 && !errors.Is(err, ErrCatalogChanged) && !errors.Is(err, ErrBackendOutdated) && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) && ctx.Err() == nil {
		fallback := q
		fallback.Sort, fallback.Profile, fallback.Revision = "popular", "", 0
		// A failed personalized source must still allow a general first page.
		// Preserve exclusions and filters, and pin the fallback for continuations.
		// The original request owns the total 25-second budget. A fallback gets
		// at most ten seconds, but cannot outlive the first request's deadline.
		fallbackCtx, fallbackCancel := context.WithTimeout(ctx, 10*time.Second)
		fallbackStart := time.Now()
		fallbackPage, fallbackErr := remote.Browse(fallbackCtx, fallback)
		timing.network += time.Since(fallbackStart)
		fallbackCancel()
		if fallbackErr == nil {
			page, err = fallbackPage, nil
			page.PersonalizationFallback = true
			s.mu.Lock()
			if snap, ok := s.browseSnapshots[snapshot]; ok {
				snap.query.Sort, snap.query.Profile = "popular", ""
				s.browseSnapshots[snapshot] = snap
			}
			s.mu.Unlock()
		}
	}
	return page, err
}

//wails:ignore
func (s *Service) HasRemoteCatalog() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.remote != nil
}

// A new authoritative claim invalidates stale claims on other visited games,
// even when those other games are outside the current result page. touched
// records, for every position this call mutates, the value it had before any
// mutation in this browse call — see browsePreparedGames — so a position
// already touched by the merge loop above is not overwritten with an
// intermediate value here. It reports whether it changed anything, so the
// caller can decide whether to persist without a full-slice diff.
func (s *Service) reconcileRemotePageLinksLocked(currentGames []Game, touched map[int]Game) bool {
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
				if id != "" {
					igdbClaims[id] = true
				}
			}
			if current.ExternalIDs.IGDB != "" {
				igdbClaims[current.ExternalIDs.IGDB] = true
			}
		}
		if len(current.ProviderLinks["steam"]) > 0 || current.ExternalIDs.Steam != "" {
			for _, id := range current.ProviderLinks["steam"] {
				if id != "" {
					claimed[id] = true
				}
			}
			if current.ExternalIDs.Steam != "" {
				claimed[current.ExternalIDs.Steam] = true
			}
		}
	}
	changedAny := false
	for _, i := range s.reconcileCandidatesLocked(claimed, igdbClaims) {
		old := s.games[i]
		before := old
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
		if _, ok := touched[i]; !ok {
			touched[i] = before
		}
		s.updateIndexAtLocked(i, before)
		changedAny = true
	}
	return changedAny
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

// reconcileCandidatesLocked lists, in catalog order, the positions that hold
// any of the claimed provider ids as a confirmed id, a provider link or a
// local id. Every other record has nothing to lose to the claims.
func (s *Service) reconcileCandidatesLocked(steamClaims, igdbClaims map[string]bool) []int {
	found := map[int]struct{}{}
	collect := func(provider string, claims map[string]bool) {
		for id := range claims {
			key := provider + ":" + strings.ToLower(id)
			for _, pos := range s.idx.byIdentity[key] {
				found[pos] = struct{}{}
			}
			for _, pos := range s.idx.byLocal[key] {
				found[pos] = struct{}{}
			}
		}
	}
	collect("steam", steamClaims)
	collect("igdb", igdbClaims)
	positions := make([]int, 0, len(found))
	for pos := range found {
		positions = append(positions, pos)
	}
	slices.Sort(positions)
	return positions
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
	aliases := make([][]string, len(items))
	for i, game := range items {
		seen := map[string]bool{}
		canonical := s.resolveIDLocked(game.ID)
		for _, key := range remoteIdentityKeys(game) {
			for _, pos := range s.idx.byIdentity[key] {
				id := s.resolveIDLocked(s.idx.games[pos].ID)
				if id != "" && id != canonical && !seen[id] && s.sameGameLocked(canonical, id) {
					seen[id] = true
					aliases[i] = append(aliases[i], id)
				}
			}
		}
		if parent := s.parentLocked(game); parent != "" && parent != canonical && !seen[parent] {
			aliases[i] = append(aliases[i], parent)
		}
	}
	return aliases
}

func remoteIdentityKeys(game Game) []string {
	keys := make([]string, 0, 4)
	forEachIdentityKey(game, func(key string) { keys = append(keys, key) })
	slices.Sort(keys)
	return slices.Compact(keys)
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

// The directory scan costs a ReadDir and a stat per cached page, so it runs at
// most once per catalogPagePruneInterval instead of after every page.
func (s *Service) pruneCatalogPagesIfDue(dir string, now time.Time) error {
	s.pruneMu.Lock()
	clock := s.clock()
	if !s.lastPrune.IsZero() && clock.Sub(s.lastPrune) < catalogPagePruneInterval {
		s.pruneMu.Unlock()
		return nil
	}
	s.lastPrune = clock
	s.pruneMu.Unlock()
	return pruneCatalogPageCache(dir, now)
}

func pruneCatalogPageCache(dir string, now time.Time) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read catalog page cache: %w", err)
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
	return nil
}

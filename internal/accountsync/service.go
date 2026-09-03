package accountsync

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"sync"
	"time"

	"typhon/internal/settings"

	"github.com/google/uuid"
	"github.com/wailsapp/wails/v3/pkg/application"
)

const (
	syncInterval       = 15 * time.Minute
	maxHydratePerSync  = 20
	maxGamesPerRequest = 500
)

var (
	ErrSyncInProgress = errors.New("accountsync: sync already in progress")
	errNotStarted     = errors.New("accountsync: service is not started")
)

type Service struct {
	store    *store
	client   *httpClient
	settings SettingsPort
	library  LibraryPort
	catalog  CatalogPort
	metadata MetadataPort

	mu      sync.Mutex
	state   syncState
	syncing bool
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
}

func NewService(
	dir, baseURL string,
	token func() (string, error),
	settingsPort SettingsPort,
	library LibraryPort,
	catalog CatalogPort,
	metadata MetadataPort,
) (*Service, error) {
	if dir == "" {
		return nil, errors.New("accountsync config dir unavailable")
	}
	if settingsPort == nil || library == nil || catalog == nil || metadata == nil {
		return nil, errors.New("accountsync requires all ports")
	}

	client, err := newHTTPClient(baseURL, token)
	if err != nil {
		return nil, err
	}

	st := newStore(dir)
	loaded, err := st.load()
	if err != nil {
		return nil, err
	}

	return &Service{
		store:    st,
		client:   client,
		settings: settingsPort,
		library:  library,
		catalog:  catalog,
		metadata: metadata,
		state:    loaded,
	}, nil
}

func (s *Service) ServiceStartup(ctx context.Context, _ application.ServiceOptions) error {
	s.mu.Lock()
	s.ctx, s.cancel = context.WithCancel(ctx)
	s.mu.Unlock()
	s.wg.Add(1)
	go s.schedule()
	return nil
}

func (s *Service) ServiceShutdown() error {
	s.mu.Lock()
	cancel := s.cancel
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	s.wg.Wait()
	return nil
}

func (s *Service) schedule() {
	defer s.wg.Done()
	s.mu.Lock()
	ctx := s.ctx
	s.mu.Unlock()
	ticker := time.NewTicker(syncInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.Sync(ctx); err != nil {
				slog.Warn("scheduled account sync failed", "error", err)
			}
		}
	}
}

func (s *Service) SyncNow() error {
	s.mu.Lock()
	base := s.ctx
	s.mu.Unlock()
	if base == nil {
		return errNotStarted
	}
	ctx, cancel := context.WithCancel(base)
	defer cancel()
	return s.Sync(ctx)
}

func (s *Service) ForgetRemote() error {
	s.mu.Lock()
	base := s.ctx
	s.mu.Unlock()
	if base == nil {
		return errNotStarted
	}
	ctx, cancel := context.WithCancel(base)
	defer cancel()

	s.mu.Lock()
	if s.syncing {
		s.mu.Unlock()
		return ErrSyncInProgress
	}
	s.syncing = true
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.syncing = false
		s.mu.Unlock()
	}()

	if err := s.client.remove(ctx); err != nil {
		return fmt.Errorf("delete account sync data: %w", err)
	}

	empty := syncState{Games: map[string]gameState{}}
	if err := s.store.save(empty); err != nil {
		return fmt.Errorf("reset accountsync state: %w", err)
	}
	s.mu.Lock()
	s.state = empty
	s.mu.Unlock()

	current := s.settings.Get()
	if current.AccountSync {
		current.AccountSync = false
		if err := s.settings.Save(current); err != nil {
			return fmt.Errorf("disable account sync after wipe: %w", err)
		}
	}
	return nil
}

//wails:ignore
func (s *Service) Sync(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !s.settings.Get().AccountSync {
		return nil
	}

	s.mu.Lock()
	if s.syncing {
		s.mu.Unlock()
		return ErrSyncInProgress
	}
	s.syncing = true
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.syncing = false
		s.mu.Unlock()
	}()

	return s.attempt(ctx, true)
}

type gameCompute struct {
	combined Game
	device   int64
}

func (s *Service) attempt(ctx context.Context, allowRetry bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	snap, err := s.client.get(ctx)
	if err != nil {
		return fmt.Errorf("fetch account sync snapshot: %w", err)
	}

	s.mu.Lock()
	st := s.state
	s.mu.Unlock()

	if snap.SettingsRevision != st.SettingsRevision {
		if err := s.applyRemoteSettings(snap.Settings); err != nil {
			return err
		}
	}

	localGames, err := s.library.Snapshot()
	if err != nil {
		return fmt.Errorf("snapshot local library: %w", err)
	}

	hydrated, deferred := s.hydrate(ctx, localGames, snap.Games)
	if hydrated > 0 {
		localGames, err = s.library.Snapshot()
		if err != nil {
			return fmt.Errorf("snapshot local library after hydration: %w", err)
		}
	}
	if deferred > 0 {
		slog.Info("account sync deferred new games to a later cycle", "count", deferred)
	}

	localByIGDB := s.indexLocalByIGDB(localGames)
	remoteByIGDB := indexRemoteByIGDB(snap.Games)

	results := make(map[string]gameCompute, len(localByIGDB))
	for igdbID, local := range localByIGDB {
		remote, hasRemote := remoteByIGDB[igdbID]
		prev := st.Games[igdbID]

		remoteSeconds := int64(0)
		remoteOwned := false
		var remoteLastPlayed *time.Time
		if hasRemote {
			remoteSeconds = remote.PlaytimeSeconds
			remoteOwned = remote.Owned
			remoteLastPlayed = remote.LastPlayedAt
		}

		combinedSeconds := local.PlaytimeSeconds
		if remoteSeconds > combinedSeconds {
			combinedSeconds = remoteSeconds
		}

		delta := local.PlaytimeSeconds - prev.Baseline
		if delta < 0 {
			delta = 0
		}

		status, statusAt := local.Status, local.StatusAt
		if hasRemote && remote.StatusAt != nil && (statusAt == nil || remote.StatusAt.After(*statusAt)) {
			status, statusAt = remote.Status, remote.StatusAt
		}
		favorite, favoriteAt := local.Favorite, local.FavoriteAt
		if hasRemote && remote.FavoriteAt != nil && (favoriteAt == nil || remote.FavoriteAt.After(*favoriteAt)) {
			favorite, favoriteAt = remote.Favorite, remote.FavoriteAt
		}

		results[igdbID] = gameCompute{
			combined: Game{
				IGDBID:          igdbID,
				CanonicalGameID: local.CanonicalGameID,
				PlaytimeSeconds: combinedSeconds,
				Owned:           local.Owned || remoteOwned,
				LastPlayed:      laterOf(local.LastPlayed, remoteLastPlayed),
				Favorite:        favorite,
				FavoriteAt:      favoriteAt,
				Status:          status,
				StatusAt:        statusAt,
			},
			device: prev.DeviceSeconds + delta,
		}
	}

	deviceID, err := deviceIDOr(st.DeviceID)
	if err != nil {
		return err
	}

	pushGames := make([]wireGame, 0, len(results))
	for igdbID, r := range results {
		id, err := strconv.ParseInt(igdbID, 10, 64)
		if err != nil {
			return fmt.Errorf("parse igdb id %q: %w", igdbID, err)
		}
		pushGames = append(pushGames, wireGame{
			IGDBID:          id,
			Owned:           r.combined.Owned,
			Favorite:        r.combined.Favorite,
			FavoriteAt:      r.combined.FavoriteAt,
			Status:          r.combined.Status,
			StatusAt:        r.combined.StatusAt,
			LastPlayedAt:    r.combined.LastPlayed,
			PlaytimeSeconds: r.device,
		})
	}
	sort.Slice(pushGames, func(i, j int) bool { return pushGames[i].IGDBID < pushGames[j].IGDBID })

	pushSettings := settings.PortableOf(s.settings.Get())
	revision := snap.SettingsRevision
	totalSkipped := 0

	for i, chunk := range chunkGames(pushGames, maxGamesPerRequest) {
		req := putRequest{
			DeviceID:         deviceID,
			SettingsRevision: revision,
			Games:            chunk,
		}
		if i == 0 {
			req.Settings = &pushSettings
		}
		resp, err := s.client.put(ctx, req)
		if err != nil {
			if errors.Is(err, ErrConflict) && allowRetry {
				return s.attempt(ctx, false)
			}
			return fmt.Errorf("push account sync: %w", err)
		}
		revision = resp.SettingsRevision
		totalSkipped += len(resp.Skipped)
	}

	if totalSkipped > 0 {
		slog.Info("account sync server skipped games the catalog does not know", "count", totalSkipped)
	}

	newState := syncState{
		DeviceID:         deviceID,
		SettingsRevision: revision,
		Games:            make(map[string]gameState, len(st.Games)+len(results)),
	}
	for id, g := range st.Games {
		newState.Games[id] = g
	}
	for igdbID, r := range results {
		newState.Games[igdbID] = gameState{DeviceSeconds: r.device, Baseline: r.combined.PlaytimeSeconds}
	}

	if err := s.store.save(newState); err != nil {
		return fmt.Errorf("save accountsync state: %w", err)
	}

	if len(results) > 0 {
		items := make([]Game, 0, len(results))
		for _, r := range results {
			items = append(items, r.combined)
		}
		if err := s.library.Apply(items); err != nil {
			return fmt.Errorf("apply merged library: %w", err)
		}
	}

	s.mu.Lock()
	s.state = newState
	s.mu.Unlock()

	return nil
}

func (s *Service) applyRemoteSettings(remote settings.Portable) error {
	current := s.settings.Get()
	merged := settings.ApplyPortable(current, remote)
	if err := s.settings.Save(merged); err != nil {
		return fmt.Errorf("save synced settings: %w", err)
	}
	return nil
}

func (s *Service) hydrate(ctx context.Context, local []Game, remote []wireGame) (hydrated, deferred int) {
	known := make(map[string]struct{}, len(local))
	for _, g := range local {
		if igdbID := s.catalog.IGDBIDOf(g.CanonicalGameID); igdbID != "" {
			known[igdbID] = struct{}{}
		}
	}

	for _, rg := range remote {
		igdbID := strconv.FormatInt(rg.IGDBID, 10)
		if _, ok := known[igdbID]; ok {
			continue
		}
		if hydrated >= maxHydratePerSync || ctx.Err() != nil {
			deferred++
			continue
		}

		title, err := s.metadata.Title(ctx, igdbID)
		if err != nil {
			slog.Warn("account sync metadata lookup failed", "igdb_id", igdbID, "error", err)
			deferred++
			continue
		}
		canonicalID, err := s.catalog.EnsureByIGDB(igdbID, title)
		if err != nil {
			slog.Warn("account sync ensure catalog entry failed", "igdb_id", igdbID, "error", err)
			deferred++
			continue
		}
		if err := s.library.Add(canonicalID, title); err != nil {
			slog.Warn("account sync add library entry failed", "igdb_id", igdbID, "error", err)
			deferred++
			continue
		}
		known[igdbID] = struct{}{}
		hydrated++
	}
	return hydrated, deferred
}

func deviceIDOr(existing string) (string, error) {
	if existing != "" {
		return existing, nil
	}
	id, err := uuid.NewRandom()
	if err != nil {
		return "", fmt.Errorf("generate accountsync device id: %w", err)
	}
	return id.String(), nil
}

func (s *Service) indexLocalByIGDB(games []Game) map[string]Game {
	out := make(map[string]Game, len(games))
	for _, g := range games {
		igdbID := s.catalog.IGDBIDOf(g.CanonicalGameID)
		if igdbID == "" {
			continue
		}
		g.IGDBID = igdbID
		out[igdbID] = g
	}
	return out
}

func indexRemoteByIGDB(games []wireGame) map[string]wireGame {
	out := make(map[string]wireGame, len(games))
	for _, g := range games {
		out[strconv.FormatInt(g.IGDBID, 10)] = g
	}
	return out
}

func laterOf(a, b *time.Time) *time.Time {
	switch {
	case a == nil:
		return b
	case b == nil:
		return a
	case b.After(*a):
		return b
	default:
		return a
	}
}

func chunkGames(games []wireGame, size int) [][]wireGame {
	if len(games) == 0 {
		return [][]wireGame{{}}
	}
	chunks := make([][]wireGame, 0, (len(games)+size-1)/size)
	for i := 0; i < len(games); i += size {
		end := i + size
		if end > len(games) {
			end = len(games)
		}
		chunks = append(chunks, games[i:end])
	}
	return chunks
}

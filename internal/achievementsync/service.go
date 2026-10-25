package achievementsync

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"typhon/internal/achievements"
	"typhon/internal/library"
	"typhon/internal/storage"
)

type GameSource interface {
	Find(id string) (library.Game, error)
}

type Config struct {
	ConfigDir string
	BaseURL   string
	Token     func() (string, error)
	// Consent reports whether the user allowed account sync; nothing is queued
	// or sent without it.
	Consent func() bool
	Games   GameSource
	// IGDBID resolves a catalog game id to its IGDB id, empty when unknown.
	IGDBID   func(catalogID string) string
	Clock    func() time.Time
	Interval time.Duration
}

type Service struct {
	client   *client
	token    func() (string, error)
	consent  func() bool
	games    GameSource
	igdbID   func(string) string
	now      func() time.Time
	interval time.Duration

	flushMu sync.Mutex

	mu     sync.Mutex
	box    *outbox
	ctx    context.Context
	cancel context.CancelFunc
	kick   chan struct{}
	wg     sync.WaitGroup
}

func New(cfg Config) (*Service, error) {
	switch {
	case cfg.ConfigDir == "" || !filepath.IsAbs(cfg.ConfigDir):
		return nil, fmt.Errorf("achievementsync: config dir %q is not an absolute path", cfg.ConfigDir)
	case cfg.Consent == nil:
		return nil, errors.New("achievementsync: consent source is required")
	case cfg.Games == nil:
		return nil, errors.New("achievementsync: game source is required")
	case cfg.IGDBID == nil:
		return nil, errors.New("achievementsync: igdb resolver is required")
	}
	cl, err := newClient(cfg.BaseURL, cfg.Token)
	if err != nil {
		return nil, err
	}
	now := cfg.Clock
	if now == nil {
		now = time.Now
	}
	interval := cfg.Interval
	if interval <= 0 {
		interval = flushInterval
	}
	return &Service{
		client:   cl,
		token:    cfg.Token,
		consent:  cfg.Consent,
		games:    cfg.Games,
		igdbID:   cfg.IGDBID,
		now:      now,
		interval: interval,
		box:      openOutbox(cfg.ConfigDir, storage.Save),
		kick:     make(chan struct{}, 1),
	}, nil
}

func (s *Service) ServiceStartup(ctx context.Context, _ application.ServiceOptions) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ctx != nil {
		return errors.New("achievementsync: already started")
	}
	runCtx, cancel := context.WithCancel(ctx)
	s.ctx, s.cancel = runCtx, cancel
	if s.box.broken != nil {
		slog.Warn("achievements outbox is unreadable, reporting is off", "error", s.box.broken)
	}
	s.wg.Add(1)
	go s.run(runCtx)
	s.wake()
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

func (s *Service) wake() {
	select {
	case s.kick <- struct{}{}:
	default:
	}
}

func (s *Service) run(ctx context.Context) {
	defer s.wg.Done()
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-s.kick:
		}
		if err := s.Flush(ctx); err != nil && ctx.Err() == nil {
			slog.Warn("achievements report not delivered, will retry", "error", err)
		}
	}
}

// Report queues fresh unlocks of a library game. A game that cannot be
// reported (no consent, not signed in, hidden, no IGDB or Steam id) is not an
// error: nothing is queued and nil comes back.
//
//wails:ignore
func (s *Service) Report(gameID, appID string, unlocks []achievements.Unlock) error {
	if len(unlocks) == 0 || !s.consent() {
		return nil
	}
	tok, err := s.token()
	if err != nil {
		return fmt.Errorf("resolve achievements token: %w", err)
	}
	if tok == "" {
		return nil
	}
	steam, ok := positiveID(appID)
	if !ok {
		slog.Debug("achievements not reported: no steam app id", "game", gameID)
		return nil
	}
	game, err := s.games.Find(gameID)
	if library.IsGameNotFound(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("find game for achievements report: %w", err)
	}
	if game.Hidden {
		return nil
	}
	igdb, ok := positiveID(s.igdbID(game.CanonicalGameID))
	if !ok {
		slog.Debug("achievements not reported: no igdb id", "game", gameID)
		return nil
	}
	now := s.now()
	items := make([]entry, 0, len(unlocks))
	for _, u := range unlocks {
		at := u.At
		if at.IsZero() {
			at = now
		}
		if u.Key == "" || now.Sub(at) > maxAge || at.Sub(now) > maxFuture {
			continue
		}
		items = append(items, entry{IGDBID: igdb, SteamAppID: steam, Key: u.Key, UnlockedAt: at.UTC()})
	}
	if len(items) == 0 {
		return nil
	}
	s.mu.Lock()
	err = s.box.add(items, now)
	s.mu.Unlock()
	if err != nil {
		return err
	}
	s.wake()
	return nil
}

// Flush sends the queue. A permanent rejection drops that batch, a retryable
// failure stops the flush and keeps everything not yet accepted.
//
//wails:ignore
func (s *Service) Flush(ctx context.Context) error {
	s.flushMu.Lock()
	defer s.flushMu.Unlock()
	if !s.consent() {
		return nil
	}
	tok, err := s.token()
	if err != nil {
		return fmt.Errorf("resolve achievements token: %w", err)
	}
	if tok == "" {
		return nil
	}

	s.mu.Lock()
	if s.box.broken != nil {
		s.mu.Unlock()
		return s.box.broken
	}
	if err := s.box.settle(nil, s.now()); err != nil {
		s.mu.Unlock()
		return err
	}
	pending := slices.Clone(s.box.items)
	s.mu.Unlock()

	for _, batch := range batches(pending) {
		if err := ctx.Err(); err != nil {
			return err
		}
		req := request{IGDBID: batch[0].IGDBID, SteamAppID: batch[0].SteamAppID}
		for _, e := range batch {
			req.Unlocks = append(req.Unlocks, Unlock{Key: e.Key, UnlockedAt: e.UnlockedAt})
		}
		err := s.client.report(ctx, req)
		var permanent *PermanentError
		switch {
		case err == nil:
		case errors.As(err, &permanent):
			slog.Warn("achievements report rejected, unlocks dropped", "igdb", req.IGDBID, "count", len(req.Unlocks), "status", permanent.Status, "code", permanent.Code)
		default:
			return err
		}
		s.mu.Lock()
		err = s.box.settle(batch, s.now())
		s.mu.Unlock()
		if err != nil {
			return err
		}
	}
	return nil
}

func positiveID(raw string) (int64, bool) {
	id, err := strconv.ParseInt(raw, 10, 64)
	return id, err == nil && id > 0
}

func batches(items []entry) [][]entry {
	type group struct{ igdb, steam int64 }
	var order []group
	by := map[group][]entry{}
	for _, e := range items {
		g := group{e.IGDBID, e.SteamAppID}
		if _, ok := by[g]; !ok {
			order = append(order, g)
		}
		by[g] = append(by[g], e)
	}
	var out [][]entry
	for _, g := range order {
		list := by[g]
		slices.SortStableFunc(list, func(a, b entry) int { return a.UnlockedAt.Compare(b.UnlockedAt) })
		for chunk := range slices.Chunk(list, maxBatch) {
			out = append(out, chunk)
		}
	}
	return out
}

package achievementsync

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"slices"
	"strconv"
	"sync"
	"time"
	"unicode/utf8"

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
	// AccountID returns the stable id of the account behind a token; an error
	// means the account is not known yet.
	AccountID func(token string) (string, error)
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
	account  func(token string) (string, error)
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
	case cfg.AccountID == nil:
		return nil, errors.New("achievementsync: account id source is required")
	case cfg.Consent == nil:
		return nil, errors.New("achievementsync: consent source is required")
	case cfg.Games == nil:
		return nil, errors.New("achievementsync: game source is required")
	case cfg.IGDBID == nil:
		return nil, errors.New("achievementsync: igdb resolver is required")
	}
	if cfg.Token == nil {
		return nil, errors.New("achievementsync: token resolver is nil")
	}
	cl, err := newClient(cfg.BaseURL)
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
		account:  cfg.AccountID,
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
	owner := s.ownerOrPending(tok)
	now := s.now()
	items := make([]entry, 0, len(unlocks))
	seen := make(map[string]struct{}, len(unlocks))
	for _, u := range unlocks {
		if !validKey(u.Key) {
			slog.Debug("achievement unlock skipped: invalid key", "game", gameID)
			continue
		}
		if _, dup := seen[u.Key]; dup {
			continue
		}
		seen[u.Key] = struct{}{}
		if u.At.IsZero() {
			slog.Debug("achievement unlock skipped: no unlock time", "game", gameID, "key", u.Key)
			continue
		}
		if now.Sub(u.At) > maxAge || u.At.Sub(now) > maxFuture {
			continue
		}
		items = append(items, entry{Owner: owner, IGDBID: igdb, SteamAppID: steam, Key: u.Key, UnlockedAt: u.At.UTC()})
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

func validKey(key string) bool {
	return key != "" && utf8.ValidString(key) && utf8.RuneCountInString(key) <= maxKeyRunes
}

// Flush sends the queue of the current account. Consent withdrawn or a
// sign-out discards the queue. A rejection the server will repeat is dropped; a
// failure of the session or the path to the server stops the flush and keeps
// everything; a server error keeps that game's batch and moves on to the next
// game.
//
//wails:ignore
func (s *Service) Flush(ctx context.Context) error {
	s.flushMu.Lock()
	defer s.flushMu.Unlock()
	if !s.consent() {
		return s.discard()
	}
	tok, err := s.token()
	if err != nil {
		return fmt.Errorf("resolve achievements token: %w", err)
	}
	if tok == "" {
		return s.discard()
	}
	owner, err := s.owner(tok)
	if err != nil {
		return err
	}

	s.mu.Lock()
	if s.box.broken != nil {
		s.mu.Unlock()
		return s.box.broken
	}
	now := s.now()
	if err := s.box.rebind(pendingOwner(tok), owner, now); err != nil {
		s.mu.Unlock()
		return err
	}
	if err := s.box.settle(nil, now); err != nil {
		s.mu.Unlock()
		return err
	}
	pending := slices.DeleteFunc(slices.Clone(s.box.items), func(e entry) bool { return e.Owner != owner })
	s.mu.Unlock()

	type game struct{ igdb, steam int64 }
	failed := map[game]struct{}{}
	var firstErr error
	for _, batch := range batches(pending) {
		if err := ctx.Err(); err != nil {
			return err
		}
		if !s.consent() {
			return s.discard()
		}
		g := game{batch[0].IGDBID, batch[0].SteamAppID}
		if _, skip := failed[g]; skip {
			continue
		}
		if current, err := s.token(); err != nil || current != tok {
			return errors.Join(errSessionChanged, err)
		}
		req := request{IGDBID: g.igdb, SteamAppID: g.steam}
		for _, e := range batch {
			req.Unlocks = append(req.Unlocks, Unlock{Key: e.Key, UnlockedAt: e.UnlockedAt})
		}
		err := s.client.report(ctx, tok, req)
		var permanent *PermanentError
		var retry *RetryableError
		switch {
		case err == nil:
		case errors.As(err, &permanent):
			slog.Warn("achievements report rejected, unlocks dropped", "igdb", req.IGDBID, "count", len(req.Unlocks), "status", permanent.Status, "code", permanent.Code)
		case errors.As(err, &retry) && !retry.Stop:
			failed[g] = struct{}{}
			if firstErr == nil {
				firstErr = err
			}
			continue
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
	return firstErr
}

// discard drops the whole queue: nothing may be sent without consent, and a
// sign-out ends the account's claim on what it had queued.
func (s *Service) discard() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.box.broken != nil || len(s.box.items) == 0 {
		return nil
	}
	if err := s.box.replace(nil); err != nil {
		return err
	}
	slog.Info("achievements outbox cleared: no consent or signed out")
	return nil
}

// ownerOrPending tags an entry with the account when it is known. While the
// account id is not known yet (the backend has not answered since sign-in and
// nothing is cached) it tags the entry with a digest of the session, which
// Flush rebinds to the account as soon as the id resolves.
func (s *Service) ownerOrPending(token string) string {
	owner, err := s.owner(token)
	if err != nil {
		slog.Debug("achievements queued before the account id is known", "error", err)
		return pendingOwner(token)
	}
	return owner
}

func pendingOwner(token string) string {
	return "session-" + fmt.Sprintf("%x", sha256.Sum256([]byte("achievementsync-session:"+token)))
}

// owner is a digest of the account id: entries stay bound to the account that
// earned them, and neither the token nor the raw id lands on disk.
func (s *Service) owner(token string) (string, error) {
	id, err := s.account(token)
	if err != nil {
		return "", fmt.Errorf("resolve achievements account: %w", err)
	}
	if id == "" {
		return "", errors.New("achievementsync: account id is empty")
	}
	return fmt.Sprintf("%x", sha256.Sum256([]byte(id))), nil
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

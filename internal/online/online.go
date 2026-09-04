package online

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"sync"
	"time"

	"typhon/internal/app"
	"typhon/internal/library"
	"typhon/internal/settings"

	"github.com/wailsapp/wails/v3/pkg/application"
)

const (
	defaultInterval  = 30 * time.Second
	defaultClearWait = 2 * time.Second
	rateLimitBackoff = 2 * defaultInterval
)

var ErrInvalidStatus = errors.New("online: unknown presence status")

var igdbIDPattern = regexp.MustCompile(`^[1-9][0-9]{0,19}$`)

func realTicker(d time.Duration) (<-chan time.Time, func()) {
	t := time.NewTicker(d)
	return t.C, t.Stop
}

type runningGame struct {
	gameID string
	seq    int64
}

type Service struct {
	client        *client
	resolveIGDBID func(canonicalGameID string) string
	settings      *settings.Service

	interval  time.Duration
	newTicker func(time.Duration) (<-chan time.Time, func())
	now       func() time.Time
	sent      chan struct{}

	mu           sync.Mutex
	status       string
	running      map[string]runningGame
	seq          int64
	healthy      bool
	healthyKnown bool
	unsupported  bool
	syncOn       bool
	retryAfter   time.Time
	unsubscribe  func()
	cancel       context.CancelFunc

	wg   sync.WaitGroup
	kick chan struct{}
}

func NewService(baseURL string, token func() (string, error), resolveIGDBID func(canonicalGameID string) string, set *settings.Service) (*Service, error) {
	if resolveIGDBID == nil {
		return nil, errors.New("online: resolveIGDBID callback is nil")
	}
	if set == nil {
		return nil, errors.New("online: settings service is nil")
	}
	cl, err := newClient(baseURL, token)
	if err != nil {
		return nil, err
	}
	return &Service{
		client:        cl,
		resolveIGDBID: resolveIGDBID,
		settings:      set,
		status:        set.GetSettings().PresenceStatus,
		interval:      defaultInterval,
		newTicker:     realTicker,
		now:           time.Now,
		running:       map[string]runningGame{},
		kick:          make(chan struct{}, 1),
	}, nil
}

func (s *Service) ServiceStartup(ctx context.Context, _ application.ServiceOptions) error {
	runCtx, cancel := context.WithCancel(ctx)

	s.mu.Lock()
	s.cancel = cancel
	s.syncOn = s.settings.GetSettings().AccountSync
	s.mu.Unlock()

	unsubscribe := s.settings.Subscribe(s.applySettings)
	s.mu.Lock()
	s.unsubscribe = unsubscribe
	s.mu.Unlock()

	s.wg.Add(1)
	go s.loop(runCtx)
	return nil
}

func (s *Service) ServiceShutdown() error {
	s.mu.Lock()
	cancel := s.cancel
	s.cancel = nil
	unsubscribe := s.unsubscribe
	s.unsubscribe = nil
	s.mu.Unlock()

	if unsubscribe != nil {
		unsubscribe()
	}
	if cancel != nil {
		cancel()
	}
	s.wg.Wait()

	if !s.enabled() {
		return nil
	}

	// ctx сервиса уже отменён; снятие присутствия — best-effort с собственным
	// коротким таймаутом, чтобы не задерживать остановку приложения.
	clearCtx, clearCancel := context.WithTimeout(context.Background(), defaultClearWait) //nolint:forbidigo // инвариант 20: сервисный ctx уже отменён, снятие присутствия — best-effort хвост завершения
	defer clearCancel()
	if err := s.client.clear(clearCtx); err != nil && !errors.Is(err, ErrSignedOut) {
		slog.Debug("clear presence", "error", err)
	}
	return nil
}

func (s *Service) SetStatus(status string) error {
	if !settings.ValidPresenceStatus(status) {
		return fmt.Errorf("%w: %s", ErrInvalidStatus, status)
	}
	next := s.settings.GetSettings()
	next.PresenceStatus = status
	if err := s.settings.SaveSettings(next); err != nil {
		return fmt.Errorf("save presence status: %w", err)
	}

	s.mu.Lock()
	changed := s.status != status
	s.status = status
	s.mu.Unlock()
	if changed {
		s.poke()
	}
	return nil
}

func (s *Service) Status() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status
}

//wails:ignore
func (s *Service) SessionStarted(game library.Game) {
	resolved := s.resolveIGDBID(game.CanonicalGameID)
	if !igdbIDPattern.MatchString(resolved) {
		resolved = ""
	}
	s.mu.Lock()
	s.seq++
	s.running[game.ID] = runningGame{gameID: resolved, seq: s.seq}
	s.mu.Unlock()
	s.poke()
}

//wails:ignore
func (s *Service) SessionStopped(gameID string) {
	s.mu.Lock()
	delete(s.running, gameID)
	s.mu.Unlock()
	s.poke()
}

func (s *Service) applySettings(next settings.Settings) {
	s.mu.Lock()
	changed := next.PresenceStatus != s.status
	if changed {
		s.status = next.PresenceStatus
	}
	syncOn := s.syncOn
	s.syncOn = next.AccountSync
	s.mu.Unlock()

	if next.AccountSync && !syncOn {
		s.Kick()
		return
	}
	if changed {
		s.poke()
	}
}

func (s *Service) enabled() bool {
	return s.settings.GetSettings().AccountSync
}

func (s *Service) poke() {
	select {
	case s.kick <- struct{}{}:
	default:
	}
}

func (s *Service) Kick() {
	s.mu.Lock()
	s.unsupported = false
	s.mu.Unlock()
	s.poke()
}

func (s *Service) loop(ctx context.Context) {
	defer s.wg.Done()

	ticks, stop := s.newTicker(s.interval)
	defer stop()

	s.report(ctx, true)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticks:
			s.report(ctx, false)
		case <-s.kick:
			s.report(ctx, false)
		}
	}
}

func (s *Service) report(ctx context.Context, forced bool) {
	s.send(ctx, forced)
	if s.sent == nil {
		return
	}
	select {
	case s.sent <- struct{}{}:
	case <-ctx.Done():
	}
}

func (s *Service) snapshot() payload {
	s.mu.Lock()
	defer s.mu.Unlock()
	var latest runningGame
	for _, g := range s.running {
		if g.seq > latest.seq {
			latest = g
		}
	}
	return payload{Status: s.status, GameID: latest.gameID, AppVersion: app.Version}
}

func (s *Service) send(ctx context.Context, forced bool) {
	if !s.enabled() {
		return
	}

	s.mu.Lock()
	if !forced {
		if s.unsupported {
			s.mu.Unlock()
			return
		}
		if !s.retryAfter.IsZero() {
			if s.now().Before(s.retryAfter) {
				s.mu.Unlock()
				return
			}
			s.retryAfter = time.Time{}
		}
	}
	s.mu.Unlock()

	err := s.client.report(ctx, s.snapshot())
	if errors.Is(err, ErrSignedOut) {
		return
	}

	if errors.Is(err, ErrUnsupported) {
		s.mu.Lock()
		alreadyUnsupported := s.unsupported
		s.unsupported = true
		s.mu.Unlock()
		if !alreadyUnsupported {
			slog.Warn("presence not supported by this server")
		}
		return
	}

	var apiErr *APIError
	rateLimited := errors.As(err, &apiErr) && apiErr.Status == http.StatusTooManyRequests

	s.mu.Lock()
	s.unsupported = false
	if rateLimited {
		s.retryAfter = s.now().Add(rateLimitBackoff)
	} else {
		s.retryAfter = time.Time{}
	}
	wasHealthy := s.healthy
	known := s.healthyKnown
	nowHealthy := err == nil
	s.healthy = nowHealthy
	s.healthyKnown = true
	s.mu.Unlock()

	switch {
	case !known:
		if !nowHealthy {
			slog.Warn("presence report failing", "error", err)
		}
	case nowHealthy && !wasHealthy:
		slog.Info("presence report recovered")
	case !nowHealthy && wasHealthy:
		slog.Warn("presence report failing", "error", err)
	case !nowHealthy:
		slog.Debug("presence report failed", "error", err)
	}
}

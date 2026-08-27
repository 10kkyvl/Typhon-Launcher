package heartbeat

import (
	"context"
	"errors"
	"log/slog"
	"regexp"
	"runtime"
	"sync"
	"time"

	"typhon/internal/account"
	"typhon/internal/app"
	"typhon/internal/clientid"
	"typhon/internal/library"

	"github.com/wailsapp/wails/v3/pkg/application"
)

const (
	stateIdle             = "idle"
	statePlaying          = "playing"
	defaultInterval       = 30 * time.Second
	defaultDisconnectWait = 2 * time.Second
)

var gameIDPattern = regexp.MustCompile(`^[1-9][0-9]{0,19}$`)

func validGameID(id string) bool {
	return gameIDPattern.MatchString(id)
}

type runningGame struct {
	gameID string
	seq    int64
}

type Service struct {
	identity      clientid.Identity
	resolveGameID func(catalogGameID string) string
	client        *client

	interval time.Duration

	mu      sync.Mutex
	running map[string]runningGame
	seq     int64

	healthy      bool
	healthyKnown bool

	cancel context.CancelFunc
	wg     sync.WaitGroup
	kick   chan struct{}
}

func NewService(id clientid.Identity, resolveGameID func(catalogGameID string) string) (*Service, error) {
	if id.InstallationID == "" || id.SessionID == "" {
		return nil, errors.New("client identity is empty")
	}
	if resolveGameID == nil {
		return nil, errors.New("resolveGameID callback is nil")
	}
	cl, err := newClient(account.BaseURL())
	if err != nil {
		return nil, err
	}
	return &Service{
		identity:      id,
		resolveGameID: resolveGameID,
		client:        cl,
		interval:      defaultInterval,
		running:       map[string]runningGame{},
		kick:          make(chan struct{}, 1),
	}, nil
}

func (s *Service) ServiceStartup(ctx context.Context, _ application.ServiceOptions) error {
	runCtx, cancel := context.WithCancel(ctx)
	s.mu.Lock()
	s.cancel = cancel
	s.mu.Unlock()

	s.wg.Add(1)
	go s.loop(runCtx)
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

	// ctx сервиса уже отменён; disconnect — best-effort с собственным
	// коротким таймаутом, чтобы не блокировать остановку приложения.
	dctx, dcancel := context.WithTimeout(context.Background(), defaultDisconnectWait) //nolint:forbidigo // инвариант 20: сервисный ctx уже отменён, disconnect — best-effort хвост завершения
	defer dcancel()
	if err := s.client.disconnect(dctx, disconnectPayload{
		SessionID:      s.identity.SessionID,
		InstallationID: s.identity.InstallationID,
	}); err != nil {
		slog.Debug("presence disconnect", "error", err)
	}
	return nil
}

//wails:ignore
func (s *Service) SessionStarted(game library.Game) {
	resolved := s.resolveGameID(game.CanonicalGameID)
	if !validGameID(resolved) {
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

func (s *Service) poke() {
	select {
	case s.kick <- struct{}{}:
	default:
	}
}

func (s *Service) loop(ctx context.Context) {
	defer s.wg.Done()
	s.send(ctx)
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.send(ctx)
		case <-s.kick:
			s.send(ctx)
		}
	}
}

func (s *Service) snapshot() (state string, gameID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.running) == 0 {
		return stateIdle, ""
	}
	var latest runningGame
	for _, g := range s.running {
		if g.seq > latest.seq {
			latest = g
		}
	}
	return statePlaying, latest.gameID
}

func (s *Service) send(ctx context.Context) {
	state, gameID := s.snapshot()
	payload := heartbeatPayload{
		SessionID:      s.identity.SessionID,
		InstallationID: s.identity.InstallationID,
		State:          state,
		AppVersion:     app.Version,
		OS:             runtime.GOOS,
		Arch:           runtime.GOARCH,
	}
	if gameID != "" {
		payload.GameID = &gameID
	}
	err := s.client.heartbeat(ctx, payload)

	s.mu.Lock()
	wasHealthy := s.healthy
	known := s.healthyKnown
	nowHealthy := err == nil
	s.healthy = nowHealthy
	s.healthyKnown = true
	s.mu.Unlock()

	switch {
	case !known:
		if !nowHealthy {
			slog.Warn("presence heartbeat failing", "error", err)
		}
	case nowHealthy && !wasHealthy:
		slog.Info("presence heartbeat recovered")
	case !nowHealthy && wasHealthy:
		slog.Warn("presence heartbeat failing", "error", err)
	case !nowHealthy:
		slog.Debug("presence heartbeat failed", "error", err)
	}
}

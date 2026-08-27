package usagestats

import (
	"context"
	"errors"
	"log/slog"
	"runtime"
	"sync"
	"time"

	"typhon/internal/account"
	"typhon/internal/app"
	"typhon/internal/clientid"

	"github.com/wailsapp/wails/v3/pkg/application"
)

const (
	defaultMaxQueue       = 500
	defaultFlushThreshold = 32
	defaultFlushInterval  = 25 * time.Second
	defaultFlushTimeout   = 3 * time.Second
	dropWarnEvery         = 50
)

type Service struct {
	identity      clientid.Identity
	client        *client
	enabled       func() bool
	resolveGameID func(catalogGameID string) string

	maxQueue       int
	flushThreshold int
	flushInterval  time.Duration
	flushTimeout   time.Duration

	mu           sync.Mutex
	queue        []Event
	disabled     bool
	droppedTotal int64
	sessionStart time.Time

	cancel context.CancelFunc
	wg     sync.WaitGroup
	kick   chan struct{}
}

func NewService(
	id clientid.Identity,
	enabled func() bool,
	resolveGameID func(catalogGameID string) string,
) (*Service, error) {
	if enabled == nil {
		return nil, errors.New("enabled callback is nil")
	}
	if resolveGameID == nil {
		return nil, errors.New("resolveGameID callback is nil")
	}
	if id.InstallationID == "" || id.SessionID == "" {
		return nil, errors.New("client identity is empty")
	}
	cl, err := newClient(account.BaseURL())
	if err != nil {
		return nil, err
	}
	return &Service{
		identity:       id,
		client:         cl,
		enabled:        enabled,
		resolveGameID:  resolveGameID,
		maxQueue:       defaultMaxQueue,
		flushThreshold: defaultFlushThreshold,
		flushInterval:  defaultFlushInterval,
		flushTimeout:   defaultFlushTimeout,
		kick:           make(chan struct{}, 1),
	}, nil
}

func (s *Service) ServiceStartup(ctx context.Context, _ application.ServiceOptions) error {
	runCtx, cancel := context.WithCancel(ctx)
	s.mu.Lock()
	s.cancel = cancel
	s.sessionStart = time.Now()
	s.mu.Unlock()

	s.Record(Event{Type: TypeLauncherSessionStarted, Timestamp: time.Now()})

	s.wg.Add(1)
	go s.loop(runCtx)
	return nil
}

func (s *Service) ServiceShutdown() error {
	s.mu.Lock()
	started := s.sessionStart
	cancel := s.cancel
	s.mu.Unlock()

	s.Record(Event{
		Type:      TypeLauncherSessionStopped,
		Timestamp: time.Now(),
		Properties: Properties{
			DurationSeconds: int64(time.Since(started).Seconds()),
		},
	})

	if cancel != nil {
		cancel()
	}
	s.wg.Wait()

	// ctx сервиса уже отменён; финальный флаш — best-effort с коротким
	// собственным таймаутом, чтобы не блокировать остановку приложения.
	fctx, fcancel := context.WithTimeout(context.Background(), s.flushTimeout) //nolint:forbidigo // инвариант 20: сервисный ctx уже отменён, финальный флаш — best-effort хвост завершения
	defer fcancel()
	s.flush(fctx)
	return nil
}

//wails:ignore
func (s *Service) SetEnabled(on bool) {
	s.mu.Lock()
	s.disabled = !on
	if !on {
		s.queue = nil
	}
	s.mu.Unlock()
}

//wails:ignore
func (s *Service) Record(ev Event) {
	if !s.enabled() {
		return
	}
	s.mu.Lock()
	disabled := s.disabled
	s.mu.Unlock()
	if disabled {
		return
	}

	if ev.Properties.GameID != "" {
		ev.Properties.GameID = s.resolveGameID(ev.Properties.GameID)
	}
	if err := validate(ev); err != nil {
		slog.Warn("usage stats event rejected", "type", ev.Type, "error", err)
		return
	}
	s.enqueue(ev)
}

func (s *Service) enqueue(ev Event) {
	s.mu.Lock()
	if len(s.queue) >= s.maxQueue {
		s.queue = s.queue[1:]
		s.droppedTotal++
		if s.droppedTotal%dropWarnEvery == 0 {
			slog.Warn("usage stats queue overflow", "dropped_total", s.droppedTotal)
		}
	}
	s.queue = append(s.queue, ev)
	shouldKick := len(s.queue) >= s.flushThreshold
	s.mu.Unlock()

	if shouldKick {
		s.poke()
	}
}

func (s *Service) poke() {
	select {
	case s.kick <- struct{}{}:
	default:
	}
}

func (s *Service) loop(ctx context.Context) {
	defer s.wg.Done()
	ticker := time.NewTicker(s.flushInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.flush(ctx)
		case <-s.kick:
			s.flush(ctx)
		}
	}
}

func (s *Service) flush(ctx context.Context) {
	s.mu.Lock()
	if s.disabled || len(s.queue) == 0 {
		s.mu.Unlock()
		return
	}
	batch := s.queue
	s.queue = nil
	s.mu.Unlock()

	payload := batchPayload{
		InstallationID: s.identity.InstallationID,
		SessionID:      s.identity.SessionID,
		AppVersion:     app.Version,
		OS:             runtime.GOOS,
		Arch:           runtime.GOARCH,
		Events:         toEventPayloads(batch),
	}
	if err := s.client.send(ctx, payload); err != nil {
		// Батч не возвращается в очередь: иначе при недоступном бэкенде
		// очередь росла бы вечно и пережила бы последующий opt-out.
		slog.Debug("usage stats flush failed", "count", len(batch), "error", err)
	}
}

func toEventPayloads(events []Event) []eventPayload {
	out := make([]eventPayload, len(events))
	for i, e := range events {
		out[i] = eventPayload(e)
	}
	return out
}

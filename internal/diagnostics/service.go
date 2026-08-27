package diagnostics

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"runtime"
	"runtime/debug"
	"sync"
	"time"

	"typhon/internal/account"
	"typhon/internal/app"
	"typhon/internal/clientid"
	"typhon/internal/usagestats"

	"github.com/google/uuid"
	"github.com/wailsapp/wails/v3/pkg/application"
)

const (
	defaultMaxQueue       = 100
	defaultRatePerMinute  = 20
	defaultRateWindow     = time.Minute
	defaultDedupWindow    = 5 * time.Minute
	defaultFlushThreshold = 10
	defaultFlushInterval  = 20 * time.Second
	defaultFlushTimeout   = 3 * time.Second
)

type Service struct {
	identity clientid.Identity
	client   *client
	enabled  func() bool

	maxQueue       int
	ratePerMinute  int
	rateWindow     time.Duration
	dedupWindow    time.Duration
	flushThreshold int
	flushInterval  time.Duration
	flushTimeout   time.Duration
	clock          func() time.Time

	mu              sync.Mutex
	queue           []reportPayload
	disabled        bool
	rateWindowStart time.Time
	rateCount       int
	seen            map[string]time.Time

	cancel context.CancelFunc
	wg     sync.WaitGroup
	kick   chan struct{}
}

func NewService(id clientid.Identity, enabled func() bool) (*Service, error) {
	if enabled == nil {
		return nil, errors.New("enabled callback is nil")
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
		maxQueue:       defaultMaxQueue,
		ratePerMinute:  defaultRatePerMinute,
		rateWindow:     defaultRateWindow,
		dedupWindow:    defaultDedupWindow,
		flushThreshold: defaultFlushThreshold,
		flushInterval:  defaultFlushInterval,
		flushTimeout:   defaultFlushTimeout,
		clock:          time.Now,
		seen:           map[string]time.Time{},
		kick:           make(chan struct{}, 1),
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
		s.seen = map[string]time.Time{}
		s.rateCount = 0
	}
	s.mu.Unlock()
}

// Capture builds a Report from an error and enqueues it for send. The
// message is derived from err.Error(); callers do not assemble error text
// by hand. A nil err is a no-op.
//
//wails:ignore
func (s *Service) Capture(component, operation string, err error, fatal bool) {
	if err == nil {
		return
	}
	s.capture(component, operation, err.Error(), string(debug.Stack()), usagestats.Classify(err), fatal)
}

// CapturePanic builds a Fatal report from a recovered panic value and its
// stack. Callers must re-panic after calling this so process crash
// semantics are unchanged; CapturePanic only reports, it never recovers on
// the caller's behalf.
//
//wails:ignore
func (s *Service) CapturePanic(component string, recovered any, stack []byte) {
	s.capture(component, "panic", fmt.Sprint(recovered), string(stack), usagestats.CodeUnknown, true)
}

// ReportClientError is the Wails-exposed endpoint the frontend calls to
// submit a captured browser error. Every field is sanitized on the Go side
// regardless of what the frontend sent; a report that fails sanitization is
// dropped, not surfaced as an error, because there is nothing the caller
// can do about a scrub failure other than not send raw data.
func (s *Service) ReportClientError(component, operation, message, stack string, fatal bool) error {
	s.capture(component, operation, message, stack, usagestats.CodeNone, fatal)
	return nil
}

func (s *Service) capture(component, operation, message, stack, errorCode string, fatal bool) {
	if !s.enabled() {
		return
	}
	s.mu.Lock()
	disabled := s.disabled
	s.mu.Unlock()
	if disabled {
		return
	}

	id, err := uuid.NewRandom()
	if err != nil {
		slog.Warn("diagnostics: generate error id", "error", err)
		return
	}

	report := Report{
		ErrorID:    id.String(),
		AppVersion: app.Version,
		OS:         runtime.GOOS,
		Arch:       runtime.GOARCH,
		Component:  component,
		Operation:  operation,
		ErrorCode:  errorCode,
		Message:    message,
		Stack:      stack,
		Timestamp:  time.Now(),
		Fatal:      fatal,
	}

	sanitized, err := sanitizeReport(report)
	if err != nil {
		slog.Warn("diagnostics: report dropped", "component", component, "operation", operation, "error", err)
		return
	}

	fingerprint := Fingerprint(sanitized.ErrorCode, sanitized.Component, sanitized.Stack)
	s.enqueue(toPayload(sanitized), fingerprint)
}

func (s *Service) enqueue(rp reportPayload, fingerprint string) {
	s.mu.Lock()
	now := s.clock()

	if now.Sub(s.rateWindowStart) >= s.rateWindow {
		s.rateWindowStart = now
		s.rateCount = 0
	}
	if s.rateCount >= s.ratePerMinute {
		s.mu.Unlock()
		return
	}

	s.pruneSeenLocked(now)
	if last, ok := s.seen[fingerprint]; ok && now.Sub(last) < s.dedupWindow {
		s.mu.Unlock()
		return
	}
	s.seen[fingerprint] = now
	s.rateCount++

	if len(s.queue) >= s.maxQueue {
		s.queue = s.queue[1:]
	}
	s.queue = append(s.queue, rp)
	shouldKick := len(s.queue) >= s.flushThreshold
	s.mu.Unlock()

	if shouldKick {
		s.poke()
	}
}

func (s *Service) pruneSeenLocked(now time.Time) {
	for fp, last := range s.seen {
		if now.Sub(last) >= s.dedupWindow {
			delete(s.seen, fp)
		}
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

	if err := s.client.send(ctx, s.identity, batch); err != nil {
		// Батч не возвращается в очередь: иначе при недоступном бэкенде
		// очередь росла бы вечно и пережила бы последующий opt-out.
		slog.Debug("diagnostics flush failed", "count", len(batch), "error", err)
	}
}

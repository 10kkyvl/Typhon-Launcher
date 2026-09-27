package compat

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
)

const (
	defaultFirstReportDelay = 2 * time.Minute
	defaultReportInterval   = 10 * time.Minute
	defaultStatsInterval    = 6 * time.Hour
	defaultSendTimeout      = 20 * time.Second
)

// Sharer — вторая половина журнала: он отдаёт наружу вердикты этой машины и
// забирает обратно общую цифру. Локальный журнал про него ничего не знает и
// работает, даже если общей статистики нет вовсе.
type Sharer struct {
	journal    *Service
	stats      *Stats
	client     *client
	clientID   string
	appVersion string

	// allowed спрашивается перед каждой отправкой, а не один раз при старте:
	// человек может выключить статистику в любой момент, и следующий круг
	// цикла обязан это увидеть.
	allowed func() bool
	env     func() Env
	resolve func(localID string) (Build, bool)

	firstReportDelay time.Duration
	reportInterval   time.Duration
	statsInterval    time.Duration
	sendTimeout      time.Duration

	mu       sync.Mutex
	lastSent string

	cancel context.CancelFunc
	wg     sync.WaitGroup
}

func NewSharer(
	journal *Service,
	stats *Stats,
	baseURL, clientID, appVersion string,
	allowed func() bool,
	env func() Env,
	resolve func(localID string) (Build, bool),
) (*Sharer, error) {
	switch {
	case journal == nil:
		return nil, errors.New("compat: журнал не задан")
	case stats == nil:
		return nil, errors.New("compat: кэш общей статистики не задан")
	case allowed == nil:
		return nil, errors.New("compat: не задан вопрос о согласии")
	case env == nil:
		return nil, errors.New("compat: не задано окружение")
	case resolve == nil:
		return nil, errors.New("compat: не задано разрешение игр в каталожные")
	case clientID == "":
		return nil, errors.New("compat: не задан идентификатор клиента")
	}
	cl, err := newClient(baseURL)
	if err != nil {
		return nil, err
	}
	return &Sharer{
		journal:          journal,
		stats:            stats,
		client:           cl,
		clientID:         clientID,
		appVersion:       appVersion,
		allowed:          allowed,
		env:              env,
		resolve:          resolve,
		firstReportDelay: defaultFirstReportDelay,
		reportInterval:   defaultReportInterval,
		statsInterval:    defaultStatsInterval,
		sendTimeout:      defaultSendTimeout,
	}, nil
}

func (s *Sharer) ServiceStartup(ctx context.Context, _ application.ServiceOptions) error {
	runCtx, cancel := context.WithCancel(ctx)
	s.cancel = cancel

	s.wg.Add(2)
	go s.reportLoop(runCtx)
	go s.statsLoop(runCtx)
	return nil
}

func (s *Sharer) ServiceShutdown() error {
	if s.cancel != nil {
		s.cancel()
	}
	s.wg.Wait()
	return nil
}

func (s *Sharer) reportLoop(ctx context.Context) {
	defer s.wg.Done()

	// Первая отправка отложена: старт лаунчера и без того занят, а журнал за
	// первые секунды всё равно не меняется.
	timer := time.NewTimer(s.firstReportDelay)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			s.sendOnce(ctx)
			timer.Reset(s.reportInterval)
		}
	}
}

func (s *Sharer) statsLoop(ctx context.Context) {
	defer s.wg.Done()

	ticker := time.NewTicker(s.statsInterval)
	defer ticker.Stop()
	s.refreshStats(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.refreshStats(ctx)
		}
	}
}

// sendOnce отправляет снимок журнала, если есть согласие и есть что слать.
// Повторная отправка того же снимка пропускается: сервер её переживёт, но
// платить трафиком за неизменившийся вердикт незачем.
func (s *Sharer) sendOnce(ctx context.Context) {
	if !s.allowed() {
		return
	}
	report := s.journal.buildReport(s.clientID, s.appVersion, s.env(), s.resolve)
	if report.Empty() {
		return
	}
	fingerprint, err := reportFingerprint(report)
	if err != nil {
		slog.Warn("compat report fingerprint", "error", err)
		return
	}
	s.mu.Lock()
	same := fingerprint == s.lastSent
	s.mu.Unlock()
	if same {
		return
	}

	sendCtx, cancel := context.WithTimeout(ctx, s.sendTimeout)
	defer cancel()
	if err := s.client.send(sendCtx, report); err != nil {
		slog.Warn("send compat report", "error", err)
		return
	}
	s.mu.Lock()
	s.lastSent = fingerprint
	s.mu.Unlock()
	slog.Info("compat report sent", "games", len(report.Games))
}

func (s *Sharer) refreshStats(ctx context.Context) {
	fetchCtx, cancel := context.WithTimeout(ctx, s.sendTimeout)
	defer cancel()

	snapshot, changed, err := s.client.fetchStats(fetchCtx, s.stats.ETag())
	if err != nil {
		slog.Warn("fetch compat stats", "error", err)
		return
	}
	if !changed {
		return
	}
	s.stats.Apply(snapshot)
	slog.Info("compat stats refreshed", "games", s.stats.Len())
}

func reportFingerprint(r Report) (string, error) {
	raw, err := json.Marshal(r)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

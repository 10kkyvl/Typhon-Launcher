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
	env     func() (Env, error)
	resolve func(localID string) (Build, bool)

	firstReportDelay time.Duration
	reportInterval   time.Duration
	statsInterval    time.Duration
	sendTimeout      time.Duration

	mu       sync.Mutex
	lastSent string
	// rejected — отпечаток отчёта, который сервер отклонил окончательно. Тот же
	// отчёт не пройдёт и со второй попытки, поэтому его не повторяют каждые
	// десять минут: хватит нового запуска или изменения журнала.
	rejected string

	// noReport пишет в лог один раз за запуск: на Windows отчёта не бывает
	// по замыслу, а на маке без версии системы это сбой, который иначе ничем
	// не виден — отказ от отправки молчит.
	noReport sync.Once

	// noEnv — то же для окружения, которое не удалось определить: причина
	// отказа в логе нужна, но повторять её каждые десять минут незачем.
	noEnv sync.Once

	cancel context.CancelFunc
	wg     sync.WaitGroup
}

func NewSharer(
	journal *Service,
	stats *Stats,
	baseURL, clientID, appVersion string,
	allowed func() bool,
	env func() (Env, error),
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
	env, err := s.env()
	if err != nil {
		s.noEnv.Do(func() {
			slog.Warn("compat report not sent: environment unknown", "error", err)
		})
		return
	}
	report, ok := s.journal.buildReport(s.clientID, s.appVersion, env, s.resolve)
	if !ok {
		s.noReport.Do(func() {
			slog.Info("compat report not sent: no macOS version", "os", env.OSVersion)
		})
		return
	}
	if report.Empty() {
		return
	}
	fingerprint, err := reportFingerprint(report)
	if err != nil {
		slog.Warn("compat report fingerprint", "error", err)
		return
	}
	s.mu.Lock()
	same := fingerprint == s.lastSent || fingerprint == s.rejected
	s.mu.Unlock()
	if same {
		return
	}

	parts, err := splitReport(report, maxReportGames, maxReportBytes)
	if err != nil {
		slog.Warn("compat report cannot be split into requests, not retrying until it changes", "error", err)
		s.reject(fingerprint)
		return
	}
	for i, part := range parts {
		if err := s.sendPart(ctx, part); err != nil {
			if rejectedForGood(err) {
				slog.Warn("compat report rejected by the server, not retrying until it changes",
					"part", i+1, "parts", len(parts), "error", err)
				s.reject(fingerprint)
				return
			}
			slog.Warn("send compat report", "part", i+1, "parts", len(parts), "error", err)
			return
		}
	}
	s.mu.Lock()
	s.lastSent = fingerprint
	s.mu.Unlock()
	slog.Info("compat report sent", "games", len(report.Games), "requests", len(parts))
}

func (s *Sharer) sendPart(ctx context.Context, part Report) error {
	sendCtx, cancel := context.WithTimeout(ctx, s.sendTimeout)
	defer cancel()
	return s.client.send(sendCtx, part)
}

func (s *Sharer) reject(fingerprint string) {
	s.mu.Lock()
	s.rejected = fingerprint
	s.mu.Unlock()
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

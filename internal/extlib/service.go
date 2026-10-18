package extlib

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"typhon/internal/catalog"
	"typhon/internal/library"
	"typhon/internal/titles"
	"typhon/internal/uierr"
)

const (
	EventImported = "extlib:imported"

	firstSyncDelay = 30 * time.Second
	syncInterval   = 5 * time.Minute
)

var (
	ErrBusy       = uierr.New("extlib.busy", "импорт игр уже выполняется")
	errNotStarted = uierr.New("extlib.not_started", "сервис импорта игр не запущен")
)

type libraryPort interface {
	ApplyExternal(ctx context.Context, items []library.ExternalGame) (library.ExternalSummary, error)
	SyncExternal(ctx context.Context, items []library.ExternalGame, present map[string]map[string]bool) (library.ExternalSummary, error)
	ExternalRefs() []library.ExternalRef
}

type catalogPort interface {
	Resolve(q catalog.Query) catalog.Match
	Provision(queries []catalog.Query) (map[string]catalog.Game, error)
}

type metadataPort interface {
	EnsureFresh(gameID string) (bool, error)
}

type ProviderResult struct {
	Provider     string    `json:"provider"`
	Found        int       `json:"found"`
	Added        int       `json:"added"`
	Updated      int       `json:"updated"`
	Skipped      []Skipped `json:"skipped"`
	NotInstalled bool      `json:"notInstalled"`
	Error        string    `json:"error"`
}

type Summary struct {
	Providers      []ProviderResult `json:"providers"`
	Added          int              `json:"added"`
	Updated        int              `json:"updated"`
	MetadataErrors []string         `json:"metadataErrors"`
}

type Service struct {
	mu       sync.Mutex
	library  libraryPort
	catalog  catalogPort
	metadata metadataPort
	scan     func(context.Context) ([]ProviderScan, error)
	timer    func(time.Duration) (<-chan time.Time, func() bool)

	syncEnabled bool
	bound       map[string]string
	lastSyncErr string

	ctx     context.Context
	cancel  context.CancelFunc
	closing bool
	running bool
	wg      sync.WaitGroup
}

func NewService(lib libraryPort, cat catalogPort, meta metadataPort) (*Service, error) {
	if lib == nil {
		return nil, errors.New("библиотека недоступна")
	}
	if cat == nil {
		return nil, errors.New("каталог игр недоступен")
	}
	if meta == nil {
		return nil, errors.New("сервис метаданных недоступен")
	}
	return &Service{library: lib, catalog: cat, metadata: meta, scan: Scan, timer: newTimer, syncEnabled: scanSupported, bound: map[string]string{}}, nil
}

func emit(name string, data any) {
	if app := application.Get(); app != nil {
		app.Event.Emit(name, data)
	}
}

func (s *Service) ServiceStartup(ctx context.Context, _ application.ServiceOptions) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	runCtx, cancel := context.WithCancel(ctx)
	s.ctx, s.cancel = runCtx, cancel
	if s.syncEnabled {
		s.wg.Add(1)
		go s.syncLoop(runCtx)
	}
	return nil
}

func newTimer(d time.Duration) (<-chan time.Time, func() bool) {
	t := time.NewTimer(d)
	return t.C, t.Stop
}

func (s *Service) ServiceShutdown() error {
	s.mu.Lock()
	s.closing = true
	cancel := s.cancel
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	s.wg.Wait()
	return nil
}

func (s *Service) begin() (context.Context, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closing || s.ctx == nil {
		return nil, errNotStarted
	}
	if s.running {
		return nil, ErrBusy
	}
	s.running = true
	s.wg.Add(1)
	return s.ctx, nil
}

func (s *Service) finish() {
	s.mu.Lock()
	s.running = false
	s.mu.Unlock()
	s.wg.Done()
}

func (s *Service) Import() (Summary, error) {
	ctx, err := s.begin()
	if err != nil {
		return Summary{}, err
	}
	defer s.finish()
	summary, err := s.run(ctx)
	if err != nil {
		return Summary{}, err
	}
	emit(EventImported, summary)
	return summary, nil
}

func (s *Service) run(ctx context.Context) (Summary, error) {
	scans, err := s.scan(ctx)
	if errors.Is(err, errors.ErrUnsupported) {
		return Summary{}, uierr.Wrap("extlib.unsupported", err)
	}
	if err != nil {
		return Summary{}, err
	}
	summary := Summary{Providers: make([]ProviderResult, 0, len(scans)), MetadataErrors: []string{}}
	var candidates []Candidate
	var failures []error
	for _, sc := range scans {
		res := ProviderResult{Provider: sc.Provider, Found: len(sc.Games), Skipped: append([]Skipped{}, sc.Skipped...)}
		switch {
		case errors.Is(sc.Err, ErrNotInstalled):
			res.NotInstalled = true
		case sc.Err != nil:
			res.Error = sc.Err.Error()
			failures = append(failures, fmt.Errorf("%s: %w", sc.Provider, sc.Err))
		default:
			candidates = append(candidates, sc.Games...)
		}
		summary.Providers = append(summary.Providers, res)
	}
	if len(failures) > 0 && len(failures) == len(scans) {
		return Summary{}, errors.Join(failures...)
	}

	items, err := s.bind(ctx, candidates)
	if err != nil {
		return Summary{}, err
	}
	applied, err := s.library.ApplyExternal(ctx, items)
	if err != nil {
		return Summary{}, fmt.Errorf("add games to the library: %w", err)
	}
	summary.Added = applied.Added
	summary.Updated = applied.Updated

	byProvider := map[string]*ProviderResult{}
	for i := range summary.Providers {
		byProvider[summary.Providers[i].Provider] = &summary.Providers[i]
	}
	canonical := make(map[string]string, len(items))
	titlesByKey := make(map[string]string, len(items))
	for _, it := range items {
		canonical[it.Provider+"\x00"+it.AppID] = it.CanonicalGameID
		titlesByKey[it.Provider+"\x00"+it.AppID] = it.Title
	}
	var fresh []string
	for _, o := range applied.Outcomes {
		res := byProvider[o.Provider]
		if res == nil {
			continue
		}
		if o.Updated {
			res.Updated++
		}
		if o.Added {
			res.Added++
			if id := canonical[o.Provider+"\x00"+o.AppID]; id != "" {
				fresh = append(fresh, o.Provider+"\x00"+o.AppID)
			}
		}
	}
	for _, sk := range applied.Skipped {
		if res := byProvider[sk.Provider]; res != nil {
			res.Skipped = append(res.Skipped, Skipped{Title: sk.Title, Reason: sk.Reason})
		}
	}
	for _, key := range fresh {
		if err := ctx.Err(); err != nil {
			return summary, err
		}
		if _, err := s.metadata.EnsureFresh(canonical[key]); err != nil {
			slog.Warn("refresh metadata for imported game", "title", titlesByKey[key], "error", err)
			summary.MetadataErrors = append(summary.MetadataErrors, titlesByKey[key]+": "+err.Error())
		}
	}
	return summary, nil
}

func (s *Service) bind(ctx context.Context, candidates []Candidate) ([]library.ExternalGame, error) {
	queries := make([]catalog.Query, len(candidates))
	matched := make([]string, len(candidates))
	var missing []catalog.Query
	for i, c := range candidates {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		q := catalog.Query{Title: c.Title, Normalized: titles.Normalize(c.Title)}
		switch c.Provider {
		case library.ProviderSteam:
			q.ExternalIDs.Steam = c.AppID
		case library.ProviderGOG:
			q.ExternalIDs.GOG = c.AppID
		}
		queries[i] = q
		if q.Normalized == "" && q.ExternalIDs.Steam == "" && q.ExternalIDs.GOG == "" {
			continue
		}
		m := s.catalog.Resolve(q)
		if m.Status == catalog.StatusMatched && m.GameID != "" {
			matched[i] = m.GameID
			continue
		}
		if q.Normalized != "" {
			missing = append(missing, q)
		}
	}
	var provisioned map[string]catalog.Game
	if len(missing) > 0 {
		var err error
		provisioned, err = s.catalog.Provision(missing)
		if err != nil {
			return nil, fmt.Errorf("provision catalog entries: %w", err)
		}
	}
	items := make([]library.ExternalGame, len(candidates))
	for i, c := range candidates {
		id := matched[i]
		if id == "" {
			if g, ok := provisioned[strings.TrimSpace(queries[i].Normalized)]; ok {
				id = g.ID
			}
		}
		items[i] = externalOf(c, id)
	}
	return items, nil
}

func externalOf(c Candidate, canonicalID string) library.ExternalGame {
	return library.ExternalGame{
		Provider:        c.Provider,
		AppID:           c.AppID,
		Title:           c.Title,
		InstallDir:      c.InstallDir,
		Executable:      c.Executable,
		LaunchURI:       c.LaunchURI,
		CanonicalGameID: canonicalID,
	}
}

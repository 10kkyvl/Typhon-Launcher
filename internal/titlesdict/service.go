package titlesdict

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"typhon/internal/account"
	"typhon/internal/redact"
	"typhon/internal/settings"
	"typhon/internal/storage"
	"typhon/internal/titles"

	"github.com/wailsapp/wails/v3/pkg/application"
)

const (
	userFileName   = "titles-dict.json"
	remoteFileName = "titles-dict-remote.json"
	remotePath     = "/v1/titles-dict"

	refreshInterval = 24 * time.Hour
	refreshTimeout  = 30 * time.Second
	retryInterval   = time.Hour
)

// Status — то, что видно снаружи о собранном словаре. Отдельные флаги на слой
// нужны, чтобы «серверный слой не приехал» не выглядело как «всё в порядке».
type Status struct {
	RemoteApplied bool       `json:"remoteApplied"`
	UserApplied   bool       `json:"userApplied"`
	RemoteETag    string     `json:"remoteEtag,omitempty"`
	RefreshedAt   *time.Time `json:"refreshedAt,omitempty"`
	LastError     string     `json:"lastError,omitempty"`
}

type Service struct {
	mu     sync.Mutex
	dir    string
	client *http.Client
	base   string
	status Status
	etag   string

	cancel  context.CancelFunc
	closing bool
	wg      sync.WaitGroup
}

// Словарь приезжает с нашего же API, а не из пользовательского фида, поэтому
// он ходит клиентом типа typhonapi, а не guard-клиентом фидов: тот намеренно
// режет loopback и приватные сети, и на локальном бэкенде словарь бы не
// загрузился ни разу. Токен не отправляется, редиректы запрещены.
func newClient() *http.Client {
	return &http.Client{
		Timeout: refreshTimeout,
		Transport: &http.Transport{
			DialContext:           (&net.Dialer{Timeout: 10 * time.Second}).DialContext,
			TLSHandshakeTimeout:   10 * time.Second,
			ResponseHeaderTimeout: 20 * time.Second,
			ExpectContinueTimeout: 5 * time.Second,
		},
		CheckRedirect: account.CheckRedirect,
	}
}

func NewService() (*Service, error) {
	dir, err := settings.ConfigDir()
	if err != nil {
		return nil, fmt.Errorf("resolve config dir: %w", err)
	}
	return NewServiceAt(dir, account.BaseURL())
}

//wails:ignore
func NewServiceAt(dir, base string) (*Service, error) {
	if dir == "" {
		return nil, errors.New("titles dictionary path unavailable")
	}
	s := &Service{dir: dir, base: base, client: newClient()}
	if err := s.rebuild(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Service) userPath() string   { return filepath.Join(s.dir, userFileName) }
func (s *Service) remotePath() string { return filepath.Join(s.dir, remoteFileName) }

// rebuild собирает словарь из трёх слоёв и подменяет активный. Испорченный
// слой не подставляет пустой словарь и не тянет за собой остальные: он
// пропускается с записью в статус и в лог, а собранное из уцелевших слоёв
// применяется.
func (s *Service) rebuild() error {
	spec, err := titles.Builtin()
	if err != nil {
		return fmt.Errorf("builtin dictionary: %w", err)
	}

	status := Status{}
	if layer, err := readLayer(s.remotePath()); err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			status.LastError = err.Error()
			slog.Warn("cached dictionary layer unusable", "error", err)
		}
	} else {
		spec = spec.Merge(layer)
		status.RemoteApplied = true
	}

	if layer, err := readLayer(s.userPath()); err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			status.LastError = err.Error()
			slog.Warn("user dictionary layer unusable", "path", s.userPath(), "error", err)
		}
	} else {
		spec = spec.Merge(layer)
		status.UserApplied = true
	}

	dict, err := titles.NewDict(spec)
	if err != nil {
		return fmt.Errorf("build dictionary: %w", err)
	}
	titles.SetActive(dict)

	s.mu.Lock()
	status.RemoteETag = s.status.RemoteETag
	status.RefreshedAt = s.status.RefreshedAt
	s.status = status
	s.mu.Unlock()

	slog.Info("title dictionary built", "remote", status.RemoteApplied, "user", status.UserApplied)
	return nil
}

func readLayer(path string) (titles.Spec, error) {
	f, err := os.Open(path)
	if err != nil {
		return titles.Spec{}, err
	}
	defer func() {
		if closeErr := f.Close(); closeErr != nil {
			slog.Warn("close dictionary layer", "path", path, "error", closeErr)
		}
	}()

	data, err := io.ReadAll(io.LimitReader(f, titles.MaxDictBytes+1))
	if err != nil {
		return titles.Spec{}, fmt.Errorf("read %s: %w", filepath.Base(path), err)
	}
	if len(data) > titles.MaxDictBytes {
		return titles.Spec{}, fmt.Errorf("%w: %s", titles.ErrDictTooLarge, filepath.Base(path))
	}
	return titles.ParseSpec(data)
}

func (s *Service) Status() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status
}

func (s *Service) ServiceStartup(ctx context.Context, _ application.ServiceOptions) error {
	startupCtx, cancel := context.WithCancel(ctx)
	s.mu.Lock()
	s.cancel = cancel
	s.mu.Unlock()
	s.wg.Add(1)
	go s.loop(startupCtx)
	return nil
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

func (s *Service) loop(ctx context.Context) {
	defer s.wg.Done()
	for {
		wait := refreshInterval
		if err := s.Refresh(ctx); err != nil {
			if ctx.Err() != nil {
				return
			}
			wait = retryInterval
			slog.Warn("dictionary refresh failed", "error", err, "retry_in", wait)
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

// Refresh забирает серверный слой. Он проходит ту же проверку, что и фиды:
// схема, размер тела, JSON и версия — и только после этого ложится на диск.
// Битый ответ оставляет рабочий кэш нетронутым.
//
//wails:ignore
func (s *Service) Refresh(ctx context.Context) error {
	if s.base == "" {
		return nil
	}
	s.mu.Lock()
	closing, etag := s.closing, s.etag
	s.mu.Unlock()
	if closing {
		return nil
	}

	url, err := account.ValidateBaseURL(s.base)
	if err != nil {
		return fmt.Errorf("dictionary url: %w", err)
	}
	url += remotePath

	reqCtx, cancel := context.WithTimeout(ctx, refreshTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("build dictionary request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("fetch dictionary: %w", redact.Error(err))
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			slog.Warn("close dictionary response", "error", closeErr)
		}
	}()

	now := time.Now()
	if resp.StatusCode == http.StatusNotModified {
		s.mu.Lock()
		s.status.RefreshedAt = &now
		s.status.LastError = ""
		s.mu.Unlock()
		return nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("dictionary server returned %d", resp.StatusCode)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, titles.MaxDictBytes+1))
	if err != nil {
		return fmt.Errorf("read dictionary body: %w", err)
	}
	if len(data) > titles.MaxDictBytes {
		return fmt.Errorf("%w: response body", titles.ErrDictTooLarge)
	}
	if _, err := titles.ParseSpec(data); err != nil {
		return fmt.Errorf("dictionary layer rejected: %w", err)
	}

	if err := storage.WriteAtomic(s.remotePath(), data); err != nil {
		return fmt.Errorf("save dictionary layer: %w", err)
	}

	s.mu.Lock()
	s.etag = resp.Header.Get("ETag")
	s.status.RemoteETag = s.etag
	s.status.RefreshedAt = &now
	s.mu.Unlock()

	return s.rebuild()
}

// Reload перечитывает слои с диска. Нужен, когда пользователь поправил свой
// файл и хочет увидеть результат, не перезапуская лаунчер.
func (s *Service) Reload() error {
	return s.rebuild()
}

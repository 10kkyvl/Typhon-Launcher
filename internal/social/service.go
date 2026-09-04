package social

import (
	"context"
	"errors"
	"log/slog"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
)

const (
	EventFriends  = "social:friends"
	EventRequests = "social:requests"

	defaultPollInterval = 60 * time.Second
)

var (
	ErrNotRunning   = errors.New("social: service is not running")
	ErrSyncDisabled = errors.New("sync_disabled")

	errEmptyUserID   = errors.New("social: user id is empty")
	errEmptyQuery    = errors.New("social: query is empty")
	errEmptyUsername = errors.New("social: username is empty")
	errEmptyCode     = errors.New("social: friend code is empty")
	errUnknownGame   = errors.New("unknown_game")
)

var igdbIDPattern = regexp.MustCompile(`^[1-9][0-9]{0,19}$`)

func emit(name string, data any) {
	if a := application.Get(); a != nil {
		a.Event.Emit(name, data)
	}
}

func realTicker(d time.Duration) (<-chan time.Time, func()) {
	t := time.NewTicker(d)
	return t.C, t.Stop
}

type SettingsPort interface {
	AccountSync() bool
	Subscribe(fn func(accountSync bool)) func()
}

type Service struct {
	client        *client
	settings      SettingsPort
	resolveIGDBID func(canonicalGameID string) string

	interval  time.Duration
	newTicker func(time.Duration) (<-chan time.Time, func())
	emitFn    func(name string, data any)
	polled    chan struct{}

	mu           sync.Mutex
	page         FriendsPage
	loaded       bool
	incoming     int
	paused       bool
	kicks        uint64
	healthy      bool
	healthyKnown bool
	syncOn       bool
	unsubscribe  func()
	ctx          context.Context
	cancel       context.CancelFunc

	wg   sync.WaitGroup
	kick chan struct{}
}

func NewService(baseURL string, token func() (string, error), settingsPort SettingsPort, resolveIGDBID func(canonicalGameID string) string) (*Service, error) {
	if resolveIGDBID == nil {
		return nil, errors.New("social: resolveIGDBID callback is nil")
	}
	if settingsPort == nil {
		return nil, errors.New("social: settings port is nil")
	}
	cl, err := newClient(baseURL, token)
	if err != nil {
		return nil, err
	}
	return &Service{
		client:        cl,
		settings:      settingsPort,
		resolveIGDBID: resolveIGDBID,
		interval:      defaultPollInterval,
		newTicker:     realTicker,
		emitFn:        emit,
		kick:          make(chan struct{}, 1),
	}, nil
}

func (s *Service) ServiceStartup(ctx context.Context, _ application.ServiceOptions) error {
	runCtx, cancel := context.WithCancel(ctx)
	s.mu.Lock()
	s.ctx = runCtx
	s.cancel = cancel
	s.syncOn = s.settings.AccountSync()
	s.mu.Unlock()

	unsubscribe := s.settings.Subscribe(s.onAccountSync)
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
	return nil
}

func (s *Service) onAccountSync(accountSync bool) {
	s.mu.Lock()
	was := s.syncOn
	s.syncOn = accountSync
	s.mu.Unlock()
	if accountSync && !was {
		s.Kick()
	}
}

func (s *Service) enabled() bool {
	return s.settings.AccountSync()
}

func (s *Service) loop(ctx context.Context) {
	defer s.wg.Done()

	ticks, stop := s.newTicker(s.interval)
	defer stop()

	s.poll(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticks:
			s.poll(ctx)
		case <-s.kick:
			s.poll(ctx)
		}
	}
}

func (s *Service) poll(ctx context.Context) {
	if s.enabled() && !s.isPaused() {
		s.health(s.refresh(ctx))
	}
	if s.polled == nil {
		return
	}
	select {
	case s.polled <- struct{}{}:
	case <-ctx.Done():
	}
}

func (s *Service) refresh(ctx context.Context) error {
	s.mu.Lock()
	started := s.kicks
	s.mu.Unlock()
	page, err := s.client.friendsPage(ctx)
	if err != nil {
		if errors.Is(err, ErrUnauthorized) {
			s.mu.Lock()
			if s.kicks == started {
				s.paused = true
			}
			s.mu.Unlock()
		}
		return err
	}

	s.mu.Lock()
	if s.kicks != started {
		s.mu.Unlock()
		return nil
	}
	s.page = page
	s.loaded = true
	s.paused = false
	changed := len(page.Incoming) != s.incoming
	s.incoming = len(page.Incoming)
	s.mu.Unlock()

	s.emitFn(EventFriends, page)
	if changed {
		s.emitFn(EventRequests, RequestsSignal{Incoming: len(page.Incoming)})
	}
	return nil
}

func (s *Service) health(err error) {
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
			slog.Warn("social refresh failing", "error", err)
		}
	case nowHealthy && !wasHealthy:
		slog.Info("social refresh recovered")
	case !nowHealthy && wasHealthy:
		slog.Warn("social refresh failing", "error", err)
	case !nowHealthy:
		slog.Debug("social refresh failed", "error", err)
	}
}

func (s *Service) isPaused() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.paused
}

func (s *Service) poke() {
	select {
	case s.kick <- struct{}{}:
	default:
	}
}

func (s *Service) runContext() (context.Context, error) {
	if !s.enabled() {
		return nil, ErrSyncDisabled
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ctx == nil {
		return nil, ErrNotRunning
	}
	return s.ctx, nil
}

func (s *Service) Kick() {
	s.mu.Lock()
	s.paused = false
	s.kicks++
	s.page = FriendsPage{}
	s.loaded = false
	s.incoming = 0
	s.mu.Unlock()
	s.poke()
}

func (s *Service) Friends() (FriendsPage, error) {
	if !s.enabled() {
		return FriendsPage{}, nil
	}
	ctx, err := s.runContext()
	if err != nil {
		return FriendsPage{}, err
	}

	s.mu.Lock()
	loaded := s.loaded
	page := s.page
	s.mu.Unlock()
	if loaded {
		return page, nil
	}

	if err := s.refresh(ctx); err != nil {
		return FriendsPage{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	return s.page, nil
}

func (s *Service) Refresh() error {
	ctx, err := s.runContext()
	if err != nil {
		return err
	}
	return s.refresh(ctx)
}

func (s *Service) SendRequest(query string) (SendResult, error) {
	trimmed := strings.TrimSpace(query)
	if trimmed == "" {
		return SendResult{}, errEmptyQuery
	}
	ctx, err := s.runContext()
	if err != nil {
		return SendResult{}, err
	}
	result, err := s.client.sendRequest(ctx, trimmed)
	if err != nil {
		return SendResult{}, err
	}
	s.poke()
	return result, nil
}

func (s *Service) Accept(userID string) error {
	return s.mutate(userID, s.client.accept)
}

func (s *Service) Decline(userID string) error {
	return s.mutate(userID, s.client.decline)
}

func (s *Service) Unfriend(userID string) error {
	return s.mutate(userID, s.client.unfriend)
}

func (s *Service) Block(userID string) error {
	return s.mutate(userID, s.client.block)
}

func (s *Service) Unblock(userID string) error {
	return s.mutate(userID, s.client.unblock)
}

func (s *Service) mutate(userID string, call func(context.Context, string) error) error {
	trimmed := strings.TrimSpace(userID)
	if trimmed == "" {
		return errEmptyUserID
	}
	ctx, err := s.runContext()
	if err != nil {
		return err
	}
	if err := call(ctx, trimmed); err != nil {
		return err
	}
	s.poke()
	return nil
}

func (s *Service) Blocks() ([]UserCard, error) {
	ctx, err := s.runContext()
	if err != nil {
		return nil, err
	}
	return s.client.blocks(ctx)
}

func (s *Service) FriendCode() (string, error) {
	ctx, err := s.runContext()
	if err != nil {
		return "", err
	}
	return s.client.friendCode(ctx)
}

func (s *Service) RotateFriendCode() (string, error) {
	ctx, err := s.runContext()
	if err != nil {
		return "", err
	}
	return s.client.rotateFriendCode(ctx)
}

func (s *Service) Profile(username string) (PublicProfile, error) {
	trimmed := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(username), "@"))
	if trimmed == "" {
		return PublicProfile{}, errEmptyUsername
	}
	ctx, err := s.runContext()
	if err != nil {
		return PublicProfile{}, err
	}
	return s.client.profile(ctx, trimmed)
}

func (s *Service) ProfileByCode(code string) (PublicProfile, error) {
	trimmed := strings.TrimSpace(code)
	if trimmed == "" {
		return PublicProfile{}, errEmptyCode
	}
	ctx, err := s.runContext()
	if err != nil {
		return PublicProfile{}, err
	}
	return s.client.profileByCode(ctx, trimmed)
}

func (s *Service) UserGames(username, cursor string) (GamesPage, error) {
	trimmed := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(username), "@"))
	if trimmed == "" {
		return GamesPage{}, errEmptyUsername
	}
	ctx, err := s.runContext()
	if err != nil {
		return GamesPage{}, err
	}
	return s.client.userGames(ctx, trimmed, strings.TrimSpace(cursor))
}

func (s *Service) GameFriends(canonicalGameID string) (GameFriends, error) {
	if strings.TrimSpace(canonicalGameID) == "" {
		return GameFriends{}, errUnknownGame
	}
	ctx, err := s.runContext()
	if err != nil {
		return GameFriends{}, err
	}
	igdbID := s.resolveIGDBID(canonicalGameID)
	if !igdbIDPattern.MatchString(igdbID) {
		return GameFriends{}, errUnknownGame
	}
	return s.client.gameFriends(ctx, igdbID)
}

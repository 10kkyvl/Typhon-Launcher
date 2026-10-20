package gamemode

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"typhon/internal/library"
	"typhon/internal/settings"
	"typhon/internal/uierr"
)

const (
	ErrorEvent = "gamemode:error"

	CodeHideNeedsTray        = "gamemode.hide_needs_tray"
	CodePowerSchemeMissing   = "gamemode.power_scheme_missing"
	CodePowerFailed          = "gamemode.power_failed"
	CodePriorityFailed       = "gamemode.priority_failed"
	CodeUnsupported          = "gamemode.unsupported"
	CodeMarkerWriteFailed    = "gamemode.marker_write_failed"
	CodeMarkerCorrupt        = "gamemode.marker_corrupt"
	CodeRestoreFailed        = "gamemode.restore_failed"
	codeFailed               = "gamemode.failed"
	NormalPriorityClass      = uint32(0x20)
	AboveNormalPriorityClass = uint32(0x8000)

	HighPerformanceScheme = "8c5e7fda-e8bf-4a96-9a85-a6e23a8c635c"

	priorityRetryDelay = 5 * time.Second
)

type Options struct {
	HighPerformance bool
	HideLauncher    bool
	RaisePriority   bool
}

func (o Options) any() bool {
	return o.HighPerformance || o.HideLauncher || o.RaisePriority
}

func Resolve(global settings.GameMode, o *library.GameModeOverride) Options {
	out := Options{
		HighPerformance: global.HighPerformance,
		HideLauncher:    global.HideLauncher,
		RaisePriority:   global.RaisePriority,
	}
	if o == nil {
		return out
	}
	if o.HighPerformance != nil {
		out.HighPerformance = *o.HighPerformance
	}
	if o.HideLauncher != nil {
		out.HideLauncher = *o.HideLauncher
	}
	if o.RaisePriority != nil {
		out.RaisePriority = *o.RaisePriority
	}
	return out
}

// ErrorInfo is the payload of ErrorEvent.
type ErrorInfo struct {
	GameID string `json:"gameId,omitempty"`
	Toggle string `json:"toggle"`
	Code   string `json:"code"`
	Detail string `json:"detail"`
}

// Status tells the UI about a startup problem that was emitted before it could listen.
type Status struct {
	Blocked bool   `json:"blocked"`
	Code    string `json:"code,omitempty"`
	Detail  string `json:"detail,omitempty"`
}

type Power interface {
	Active() (string, error)
	Has(scheme string) (bool, error)
	Set(scheme string) error
}

type Priority interface {
	Get(pid uint32) (uint32, error)
	Set(pid, class uint32) error
	Created(pid uint32) (time.Time, error)
}

// CanHide needs the tray: without it nothing could bring a hidden window back.
type Window interface {
	CanHide() bool
	Visible() bool
	Hide()
	Show()
}

type Deps struct {
	Settings    func() settings.Settings
	PID         func(gameID string) (uint32, bool)
	Window      Window
	Emit        func(name string, data any)
	Power       Power
	Priority    Priority
	Descendants func(root uint32) ([]uint32, error)
	After       func(d time.Duration) <-chan time.Time
}

type event struct {
	start  bool
	retry  bool
	gameID string
	game   library.Game
}

type tracked struct {
	prev    uint32
	created time.Time
}

type session struct {
	opts         Options
	power        bool
	hide         bool
	rootCreated  time.Time
	raised       map[uint32]tracked
	skipped      map[uint32]struct{}
	prioReported bool
}

type Service struct {
	deps  Deps
	store *store

	qmu   sync.Mutex
	queue []event
	wake  chan struct{}

	procMu sync.Mutex
	active map[string]*session
	counts toggles
	mark   marker
	hidden bool

	statusMu sync.RWMutex
	blocked  error

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

type toggles struct {
	power, hide int
}

func New(dir string, deps Deps) (*Service, error) {
	if dir == "" {
		return nil, errors.New("gamemode: empty config dir")
	}
	if deps.Settings == nil || deps.PID == nil || deps.Window == nil || deps.Emit == nil ||
		deps.Power == nil || deps.Priority == nil || deps.Descendants == nil || deps.After == nil {
		return nil, errors.New("gamemode: incomplete dependencies")
	}
	return &Service{
		deps:   deps,
		store:  newStore(dir),
		wake:   make(chan struct{}, 1),
		active: map[string]*session{},
	}, nil
}

func (s *Service) Status() Status {
	s.statusMu.RLock()
	defer s.statusMu.RUnlock()
	if s.blocked == nil {
		return Status{}
	}
	return Status{Blocked: true, Code: uierr.Code(s.blocked), Detail: s.blocked.Error()}
}

func (s *Service) isBlocked() bool {
	return s.blockedErr() != nil
}

func (s *Service) blockedErr() error {
	s.statusMu.RLock()
	defer s.statusMu.RUnlock()
	return s.blocked
}

//wails:ignore
func (s *Service) SessionStarted(game library.Game) {
	s.enqueue(event{start: true, gameID: game.ID, game: game})
}

//wails:ignore
func (s *Service) SessionStopped(gameID string) {
	s.enqueue(event{gameID: gameID})
}

func (s *Service) enqueue(ev event) {
	s.qmu.Lock()
	s.queue = append(s.queue, ev)
	s.qmu.Unlock()
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

func (s *Service) take() []event {
	s.qmu.Lock()
	defer s.qmu.Unlock()
	out := s.queue
	s.queue = nil
	return out
}

func (s *Service) report(gameID, toggle string, err error) {
	code := uierr.Code(err)
	if code == "" {
		code = codeFailed
	}
	slog.Warn("game mode", "game", gameID, "toggle", toggle, "code", code, "error", err)
	s.deps.Emit(ErrorEvent, ErrorInfo{GameID: gameID, Toggle: toggle, Code: code, Detail: err.Error()})
}

// window runs a window call off the worker: the window needs the main thread,
// which is blocked inside ServiceShutdown, so the wait ends with the context
// and the call is left to finish on its own.
func (s *Service) window(fn func()) bool {
	done := make(chan struct{})
	go func() {
		fn()
		close(done)
	}()
	select {
	case <-done:
		return true
	case <-s.ctx.Done():
		return false
	}
}

func (s *Service) windowVisible() (visible, ok bool) {
	ok = s.window(func() { visible = s.deps.Window.Visible() })
	return visible, ok
}

func (s *Service) ServiceStartup(ctx context.Context, _ application.ServiceOptions) error {
	return s.start(ctx)
}

func (s *Service) start(ctx context.Context) error {
	s.procMu.Lock()
	s.ctx, s.cancel = context.WithCancel(ctx)
	s.recoverLocked()
	s.procMu.Unlock()

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		for {
			select {
			case <-s.ctx.Done():
				return
			case <-s.wake:
				s.drain()
			}
		}
	}()
	return nil
}

func (s *Service) ServiceShutdown() error {
	if s.cancel != nil {
		s.cancel()
	}
	s.wg.Wait()
	s.procMu.Lock()
	defer s.procMu.Unlock()
	return s.shutdownLocked()
}

func (s *Service) recoverLocked() {
	m, found, err := s.store.load()
	if err != nil {
		blocked := uierr.Wrap(CodeMarkerCorrupt, fmt.Errorf("game mode marker: %w", err))
		s.statusMu.Lock()
		s.blocked = blocked
		s.statusMu.Unlock()
		s.report("", "marker", blocked)
		return
	}
	if !found {
		return
	}
	s.mark = m
	slog.Info("game mode: restoring after an unclean exit")
	if err := s.restoreAllLocked(""); err != nil {
		slog.Warn("game mode: recovery left a marker behind", "error", err)
	}
}

func (s *Service) drain() {
	s.procMu.Lock()
	defer s.procMu.Unlock()
	for _, ev := range s.take() {
		switch {
		case ev.retry:
			if sess, ok := s.active[ev.gameID]; ok && sess.opts.RaisePriority {
				s.raiseLocked(ev.gameID, sess)
			}
		case ev.start:
			s.startLocked(ev)
		default:
			s.stopLocked(ev.gameID)
		}
	}
}

func (s *Service) startLocked(ev event) {
	if _, dup := s.active[ev.gameID]; dup {
		return
	}
	opts := Resolve(s.deps.Settings().GameMode, ev.game.GameMode)
	if !opts.any() {
		return
	}
	pid, ok := s.deps.PID(ev.gameID)
	if !ok {
		return
	}
	if blocked := s.blockedErr(); blocked != nil && (opts.HighPerformance || opts.HideLauncher) {
		s.report(ev.gameID, "marker", blocked)
		opts.HighPerformance = false
		opts.HideLauncher = false
		if !opts.any() {
			return
		}
	}
	sess := &session{opts: opts, raised: map[uint32]tracked{}, skipped: map[uint32]struct{}{}}
	s.active[ev.gameID] = sess
	if opts.HighPerformance {
		sess.power = s.acquirePower(ev.gameID)
	}
	if opts.HideLauncher {
		if s.deps.Window.CanHide() {
			sess.hide = s.acquireHide(ev.gameID)
		} else {
			s.report(ev.gameID, "hide", uierr.New(CodeHideNeedsTray, "the launcher can only be hidden while the tray icon is enabled"))
		}
	}
	if opts.RaisePriority {
		s.raiseWithPID(ev.gameID, sess, pid)
		s.scheduleRetry(ev.gameID)
	}
}

func (s *Service) stopLocked(gameID string) {
	sess, ok := s.active[gameID]
	if !ok {
		return
	}
	delete(s.active, gameID)
	s.reportPriority(gameID, sess, s.restorePriority(sess))
	if sess.power {
		s.releasePower(gameID)
	}
	if sess.hide {
		s.releaseHide(gameID)
	}
}

// shutdownLocked never touches the window: the process is going away with it.
func (s *Service) shutdownLocked() error {
	for id, sess := range s.active {
		s.reportPriority(id, sess, s.restorePriority(sess))
	}
	s.active = map[string]*session{}
	s.hidden = false
	s.mark.Hide = false
	return s.restoreAllLocked("")
}

func (s *Service) scheduleRetry(gameID string) {
	if s.ctx == nil {
		return
	}
	ch := s.deps.After(priorityRetryDelay)
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		select {
		case <-ch:
			s.enqueue(event{retry: true, gameID: gameID})
		case <-s.ctx.Done():
		}
	}()
}

func (s *Service) raiseLocked(gameID string, sess *session) {
	pid, ok := s.deps.PID(gameID)
	if !ok {
		return
	}
	s.raiseWithPID(gameID, sess, pid)
}

func (s *Service) reportPriority(gameID string, sess *session, errs []error) {
	if len(errs) == 0 || sess.prioReported {
		return
	}
	sess.prioReported = true
	s.report(gameID, "priority", uierr.Wrap(CodePriorityFailed, errors.Join(errs...)))
}

func (s *Service) raiseWithPID(gameID string, sess *session, pid uint32) {
	var errs []error
	defer func() { s.reportPriority(gameID, sess, errs) }()

	rootCreated, err := s.deps.Priority.Created(pid)
	if err != nil {
		errs = append(errs, fmt.Errorf("creation time of %d: %w", pid, err))
		return
	}
	if sess.rootCreated.IsZero() {
		sess.rootCreated = rootCreated
	} else if !rootCreated.Equal(sess.rootCreated) {
		return
	}
	pids := []uint32{pid}
	children, err := s.deps.Descendants(pid)
	if err != nil {
		errs = append(errs, fmt.Errorf("list child processes: %w", err))
	}
	pids = append(pids, children...)
	for _, p := range pids {
		if _, done := sess.raised[p]; done {
			continue
		}
		if _, skip := sess.skipped[p]; skip {
			continue
		}
		created := rootCreated
		if p != pid {
			created, err = s.deps.Priority.Created(p)
			if err != nil {
				sess.skipped[p] = struct{}{}
				errs = append(errs, fmt.Errorf("creation time of %d: %w", p, err))
				continue
			}
			if created.Before(sess.rootCreated) {
				sess.skipped[p] = struct{}{}
				continue
			}
		}
		prev, err := s.deps.Priority.Get(p)
		if err != nil {
			sess.skipped[p] = struct{}{}
			errs = append(errs, fmt.Errorf("read priority of %d: %w", p, err))
			continue
		}
		if prev != NormalPriorityClass {
			sess.skipped[p] = struct{}{}
			continue
		}
		if err := s.deps.Priority.Set(p, AboveNormalPriorityClass); err != nil {
			sess.skipped[p] = struct{}{}
			errs = append(errs, fmt.Errorf("raise priority of %d: %w", p, err))
			continue
		}
		sess.raised[p] = tracked{prev: prev, created: created}
	}
}

// restorePriority skips a process that is gone or whose pid now belongs to a
// different process.
func (s *Service) restorePriority(sess *session) []error {
	var errs []error
	for pid, t := range sess.raised {
		created, err := s.deps.Priority.Created(pid)
		if err != nil || !created.Equal(t.created) {
			continue
		}
		cur, err := s.deps.Priority.Get(pid)
		if err != nil || cur != AboveNormalPriorityClass {
			continue
		}
		if err := s.deps.Priority.Set(pid, t.prev); err != nil {
			errs = append(errs, fmt.Errorf("restore priority of %d: %w", pid, err))
		}
	}
	sess.raised = map[uint32]tracked{}
	return errs
}

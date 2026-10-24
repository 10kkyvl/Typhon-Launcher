package cloudsaves

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/wailsapp/wails/v3/pkg/application"

	"typhon/internal/account"
	"typhon/internal/library"
	"typhon/internal/savebackup"
	"typhon/internal/settings"
	"typhon/internal/storage"
	"typhon/internal/uierr"
)

const (
	defaultInterval = 5 * time.Minute
	defaultDebounce = 3 * time.Second
	defaultRetry    = 30 * time.Second
)

type saveClient interface {
	ListSaves(ctx context.Context) (account.SaveList, error)
	UploadSave(ctx context.Context, gameID string, body io.ReadSeeker, size int64, up account.SaveUpload) (account.Save, error)
	DownloadSave(ctx context.Context, gameID string, dst io.Writer, maxBytes int64) (account.SaveDownload, error)
	DeleteSave(ctx context.Context, gameID string) error
}

type libraryPort interface {
	GetGames() []library.Game
	LocateSaves(ctx context.Context, id string) (library.SavesResult, error)
	GetRunningGames() []string
}

type backupPort interface {
	SnapshotPath(ctx context.Context, gameID, sourcePath string, kind savebackup.Kind) (savebackup.Snapshot, error)
	Restore(ctx context.Context, gameID, snapshotID string) error
}

type Service struct {
	store     store
	config    func() settings.Settings
	token     func() (string, error)
	accountID func(token string) (string, error)
	client    saveClient
	lib       libraryPort
	backups   backupPort
	emit      func(Event)
	now       func() time.Time
	interval  time.Duration
	debounce  time.Duration
	// retryDelay is the wait before a pass that could not tell whose account
	// it is runs again.
	retryDelay time.Duration

	// opMu serializes everything that reads or changes the cloud and the
	// recorded state: a pass, a manual sync, a conflict resolution, a delete.
	opMu  sync.Mutex
	cache map[string]cachedTree
	// refused holds, per game, the scan that already proved too large for the
	// cloud; it is read and written under opMu.
	refused map[string]refusal
	build   func(ctx context.Context, dir string, files []treeFile, maxBytes int64) (archive, error)

	mu       sync.Mutex
	ctx      context.Context
	cancel   context.CancelFunc
	closing  bool
	wg       sync.WaitGroup
	owner    string
	loaded   bool
	loadErr  error
	state    syncState
	statuses map[string]GameStatus
	usage    account.SaveList
	dirty    map[string]struct{}
	global   string
	pending  string
	enabled  bool
	wake     chan struct{}
}

type refusal struct {
	sig   string
	limit int64
	err   error
}

type cachedTree struct {
	sig    string
	digest string
}

func NewService(
	configDir string,
	config func() settings.Settings,
	token func() (string, error),
	accountID func(token string) (string, error),
	client saveClient,
	lib libraryPort,
	backups backupPort,
) (*Service, error) {
	switch {
	case configDir == "" || !filepath.IsAbs(configDir):
		return nil, fmt.Errorf("cloudsaves: config dir %q is not an absolute path", configDir)
	case config == nil:
		return nil, errors.New("cloudsaves: settings source is required")
	case token == nil:
		return nil, errors.New("cloudsaves: token source is required")
	case accountID == nil:
		return nil, errors.New("cloudsaves: account id source is required")
	case client == nil:
		return nil, errors.New("cloudsaves: cloud client is required")
	case lib == nil:
		return nil, errors.New("cloudsaves: library is required")
	case backups == nil:
		return nil, errors.New("cloudsaves: save backups are required")
	}
	return &Service{
		store:      store{dir: configDir, write: storage.Save},
		config:     config,
		token:      token,
		accountID:  accountID,
		client:     client,
		lib:        lib,
		backups:    backups,
		emit:       emitEvent,
		now:        time.Now,
		interval:   defaultInterval,
		debounce:   defaultDebounce,
		retryDelay: defaultRetry,
		enabled:    config().CloudSaves,
		cache:      map[string]cachedTree{},
		refused:    map[string]refusal{},
		build:      buildArchive,
		statuses:   map[string]GameStatus{},
		dirty:      map[string]struct{}{},
		wake:       make(chan struct{}, 1),
	}, nil
}

func emitEvent(ev Event) {
	if app := application.Get(); app != nil {
		app.Event.Emit(EventName, ev)
	}
}

func (s *Service) ServiceStartup(ctx context.Context, _ application.ServiceOptions) error {
	s.mu.Lock()
	if s.ctx != nil {
		s.mu.Unlock()
		return errors.New("cloudsaves: already started")
	}
	runCtx, cancel := context.WithCancel(ctx)
	s.ctx, s.cancel = runCtx, cancel
	s.wg.Add(1)
	s.mu.Unlock()

	// Temporary archives of an interrupted run are worthless: the next pass
	// rebuilds or re-downloads them, so a failed sweep only wastes disk.
	if err := os.RemoveAll(s.store.tmpDir()); err != nil {
		slog.Warn("sweep cloud saves temporary files", "error", err)
	}
	go func() {
		defer s.wg.Done()
		s.loop(runCtx)
	}()
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

func (s *Service) begin() (context.Context, func(), error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ctx == nil || s.closing {
		return nil, nil, errNotStarted
	}
	s.wg.Add(1)
	ctx, cancel := context.WithCancel(s.ctx)
	return ctx, func() {
		cancel()
		s.wg.Done()
	}, nil
}

func (s *Service) loop(ctx context.Context) {
	tick := time.NewTicker(s.interval)
	defer tick.Stop()
	var timer, retryTimer *time.Timer
	var fire, retryC <-chan time.Time
	defer func() {
		if timer != nil {
			timer.Stop()
		}
		if retryTimer != nil {
			retryTimer.Stop()
		}
	}()
	run := func(only map[string]struct{}) {
		if !s.pass(ctx, only) {
			return
		}
		if retryTimer != nil {
			retryTimer.Stop()
		}
		retryTimer = time.NewTimer(s.retryDelay)
		retryC = retryTimer.C
	}
	run(nil)
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			run(nil)
		case <-retryC:
			retryC = nil
			run(nil)
		case <-s.wake:
			if timer != nil {
				timer.Stop()
			}
			timer = time.NewTimer(s.debounce)
			fire = timer.C
		case <-fire:
			fire = nil
			run(s.takeDirty())
		}
	}
}

// Nudge asks for a sync of one game soon; an empty id asks for every game. The
// pass runs after the debounce, so a burst of requests costs one pass.
//
//wails:ignore
func (s *Service) Nudge(gameID string) {
	s.mu.Lock()
	if s.ctx == nil || s.closing {
		s.mu.Unlock()
		return
	}
	s.dirty[gameID] = struct{}{}
	s.mu.Unlock()
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

func (s *Service) takeDirty() map[string]struct{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	taken := s.dirty
	s.dirty = map[string]struct{}{}
	if _, all := taken[""]; all {
		return nil
	}
	return taken
}

// SessionStarted and SessionStopped make the service a library session
// watcher. The library calls them under its own lock, so they only queue a
// request and return.
//
//wails:ignore
func (s *Service) SessionStarted(library.Game) {}

//wails:ignore
func (s *Service) SessionStopped(gameID string) {
	if !s.config().CloudSaves {
		return
	}
	s.Nudge(gameID)
}

func (s *Service) Status() Status {
	cfg := s.config()
	st := Status{Enabled: cfg.CloudSaves, Games: []GameStatus{}}
	token, err := s.token()
	switch {
	case err != nil:
		st.Error = codeToken
	case token != "":
		st.SignedIn = true
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	st.UsedBytes, st.QuotaBytes, st.MaxBytes = s.usage.UsedBytes, s.usage.QuotaBytes, s.usage.MaxBytes
	st.Pending = s.pending != ""
	switch {
	case s.loadErr != nil:
		st.Error = codeStateBroken
	case st.Error == "":
		st.Error = s.global
	}
	for _, g := range s.statuses {
		st.Games = append(st.Games, g)
	}
	slices.SortFunc(st.Games, func(a, b GameStatus) int {
		switch {
		case a.GameID < b.GameID:
			return -1
		case a.GameID > b.GameID:
			return 1
		}
		return 0
	})
	return st
}

type run struct {
	token    string
	owner    string
	stateErr error
	list     account.SaveList
	remote   map[string]account.Save
}

// identify checks the gates and loads the state of the signed-in account.
// An unreadable state does not fail it: callers that only delete from the
// cloud may go on, those that need the digests look at stateErr.
func (s *Service) identify(needEnabled bool) (*run, error) {
	if needEnabled && !s.config().CloudSaves {
		return nil, errDisabled
	}
	token, err := s.token()
	if err != nil {
		return nil, uierr.Wrap(codeToken, fmt.Errorf("доступ к сохранённому входу: %w", err))
	}
	if token == "" {
		return nil, errNotSignedIn
	}
	owner, err := s.accountID(token)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errAccountUnk, err)
	}
	if owner == "" {
		return nil, errAccountUnk
	}
	r := &run{token: token, owner: owner}
	r.stateErr = s.loadState(owner)
	return r, nil
}

func (s *Service) prepare(ctx context.Context, needEnabled bool) (*run, error) {
	r, err := s.identify(needEnabled)
	if err != nil {
		return nil, err
	}
	if r.stateErr != nil {
		return r, r.stateErr
	}
	list, err := s.client.ListSaves(ctx)
	if err != nil {
		return nil, err
	}
	r.list = list
	r.remote = make(map[string]account.Save, len(list.Games))
	for _, g := range list.Games {
		r.remote[g.GameID] = g
	}
	s.mu.Lock()
	s.usage = list
	s.mu.Unlock()
	return r, nil
}

func (s *Service) loadState(owner string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.loaded && s.owner == owner && s.loadErr == nil {
		return nil
	}
	if s.owner != owner {
		s.statuses = map[string]GameStatus{}
		s.usage = account.SaveList{}
	}
	st, err := s.store.load(owner)
	s.owner, s.loaded = owner, true
	if err != nil {
		s.loadErr = fmt.Errorf("%w: %w", errStateBroken, err)
		s.state = syncState{}
		return s.loadErr
	}
	s.loadErr = nil
	if st.DeviceID == "" {
		st.DeviceID = uuid.NewString()
	}
	s.state = st
	return nil
}

func (s *Service) gameState(key string) gameState {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state.Games[key]
}

func (s *Service) deviceID() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state.DeviceID
}

// commit changes one game's record in memory, writes the state and, if the
// write fails, puts memory back: a state that exists only in memory would let
// the next successful write record something the disk never confirmed.
func (s *Service) commit(key string, change func(*gameState)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.loadErr != nil {
		return s.loadErr
	}
	prev, had := s.state.Games[key]
	next := prev
	change(&next)
	s.state.Games[key] = next
	if err := s.store.save(s.owner, s.state); err != nil {
		if had {
			s.state.Games[key] = prev
		} else {
			delete(s.state.Games, key)
		}
		return fmt.Errorf("save cloud saves state: %w", err)
	}
	return nil
}

func (s *Service) drop(keys []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.loadErr != nil {
		return s.loadErr
	}
	prev := make(map[string]gameState, len(keys))
	for _, k := range keys {
		if g, ok := s.state.Games[k]; ok {
			prev[k] = g
			delete(s.state.Games, k)
		}
	}
	if len(prev) == 0 {
		return nil
	}
	if err := s.store.save(s.owner, s.state); err != nil {
		for k, g := range prev {
			s.state.Games[k] = g
		}
		return fmt.Errorf("save cloud saves state: %w", err)
	}
	return nil
}

func (s *Service) findGame(id string) (library.Game, error) {
	for _, g := range s.lib.GetGames() {
		if g.ID == id {
			return g, nil
		}
	}
	return library.Game{}, errGameNotFound
}

func eligible(g library.Game) bool {
	return g.CanonicalGameID != "" && !g.Uninstalled && !g.Archived
}

// pass reports whether it should run again soon: it could not tell whose
// account is signed in yet, which is expected right after the app starts.
func (s *Service) pass(ctx context.Context, only map[string]struct{}) (retry bool) {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	if ctx.Err() != nil {
		return false
	}
	r, err := s.prepare(ctx, true)
	switch {
	case errors.Is(err, errDisabled), errors.Is(err, errNotSignedIn):
		return false
	case errors.Is(err, errAccountUnk):
		if ctx.Err() == nil {
			s.record("", outcome{state: StatePending, err: err})
		}
		return ctx.Err() == nil
	case err != nil:
		if ctx.Err() == nil {
			s.record("", fail(err))
		}
		return false
	}
	s.mu.Lock()
	s.global, s.pending = "", ""
	s.mu.Unlock()
	s.seedConflicts()
	games := s.lib.GetGames()
	dups := duplicateKeys(games)
	for _, g := range games {
		if ctx.Err() != nil {
			return false
		}
		if !eligible(g) {
			continue
		}
		if only != nil {
			if _, ok := only[g.ID]; !ok {
				continue
			}
		}
		if dups[g.CanonicalGameID] {
			s.record(g.ID, fail(errDuplicate))
			continue
		}
		out := s.syncGame(ctx, r, g, modeAuto)
		if ctx.Err() != nil {
			return false
		}
		s.record(g.ID, out)
	}
	return false
}

// duplicateKeys returns the catalog ids that more than one live library entry
// carries. Two entries would overwrite each other's cloud copy every pass, so
// neither is synced.
func duplicateKeys(games []library.Game) map[string]bool {
	count := map[string]int{}
	for _, g := range games {
		if eligible(g) {
			count[g.CanonicalGameID]++
		}
	}
	dups := map[string]bool{}
	for key, n := range count {
		if n > 1 {
			dups[key] = true
		}
	}
	return dups
}

// SettingsChanged starts a sync when the player turns cloud saves on, instead
// of leaving them to the next scheduled pass.
//
//wails:ignore
func (s *Service) SettingsChanged(cfg settings.Settings) {
	s.mu.Lock()
	was := s.enabled
	s.enabled = cfg.CloudSaves
	s.mu.Unlock()
	if !was && cfg.CloudSaves {
		s.Nudge("")
	}
}

// seedConflicts shows conflicts recorded by an earlier run before the pass
// reaches their games.
func (s *Service) seedConflicts() {
	byKey := map[string]string{}
	for _, g := range s.lib.GetGames() {
		if g.CanonicalGameID != "" {
			byKey[g.CanonicalGameID] = g.ID
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for key, gs := range s.state.Games {
		id, ok := byKey[key]
		if !ok || gs.Conflict == nil {
			continue
		}
		if _, known := s.statuses[id]; known {
			continue
		}
		s.statuses[id] = GameStatus{GameID: id, State: StateConflict, At: s.now(), SyncedAt: gs.SyncedAt, Conflict: gs.Conflict}
	}
}

type outcome struct {
	state    State
	err      error
	code     string
	changed  bool
	syncedAt time.Time
	conflict *Conflict
}

func (o outcome) codeOf() string {
	if o.code != "" {
		return o.code
	}
	if o.err != nil {
		return classify(o.err)
	}
	return ""
}

func (s *Service) record(gameID string, o outcome) {
	code := o.codeOf()
	msg := ""
	if o.err != nil {
		msg = o.err.Error()
	}
	now := s.now()
	s.mu.Lock()
	prev, had := s.statuses[gameID]
	changed := o.changed || !had || prev.State != o.state || prev.Code != code
	if gameID == "" {
		switch o.state {
		case StateFailed:
			changed = s.global != code
			s.global = code
		case StatePending:
			changed = s.pending != code
			s.pending = code
		default:
			changed = true
		}
	} else {
		st := GameStatus{GameID: gameID, State: o.state, Code: code, Message: msg, At: now, SyncedAt: o.syncedAt, Conflict: o.conflict}
		if st.SyncedAt.IsZero() && had {
			st.SyncedAt = prev.SyncedAt
		}
		s.statuses[gameID] = st
	}
	used, quota := s.usage.UsedBytes, s.usage.QuotaBytes
	s.mu.Unlock()
	if o.state == StateFailed && (changed || (gameID != "" && prev.Message != msg)) {
		slog.Warn("cloud saves", "game", gameID, "code", code, "error", o.err)
	}
	if changed {
		s.emit(Event{GameID: gameID, Status: o.state, Code: code, Message: msg, UsedBytes: used, QuotaBytes: quota})
	}
}

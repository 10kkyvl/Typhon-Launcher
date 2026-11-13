package cloudsaves

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"typhon/internal/account"
	"typhon/internal/library"
)

// launchCheck is the one look at a game's cloud copy that every launch of
// the game waiting at the same time shares, so a second Play never starts a
// second transfer. err is written once, before done is closed.
type launchCheck struct {
	gameID string
	key    string
	cancel context.CancelFunc
	done   chan struct{}
	err    error

	// Guarded by Service.mu.
	waiters int
	// known is the block the check has already proved while it waits for opMu
	// to act on it: ErrSyncing for a newer cloud copy, ErrConflictUnresolved
	// for a change on both sides. From then on the check goes on in the
	// background whatever its launches do.
	known error
	// abandoned is set, together with the cancel, when the last launch stops
	// waiting while the check has proved nothing yet.
	abandoned bool
}

func blocked(err error) error { return &library.LaunchBlockedError{Err: err} }

func checkError(err error) error {
	return fmt.Errorf("cloud saves check before launch: %w", err)
}

// SessionStarting makes the service a library.LaunchHook: a game whose saves
// are unsafe does not start. A conflict already recorded, or a download of
// the game's cloud copy in progress, blocks at once and without the network.
// Otherwise one listing tells whether the cloud holds a copy this device has
// not seen: an unchanged local copy is replaced before the game starts, a
// copy changed on both sides is recorded as a conflict and blocks. When the
// cloud fails or does not answer within launchTimeout before anything is
// known, the game starts and the returned plain error carries the cause into
// the library's log.
//
// While a check of the game may still replace its saves, every launch of the
// game waits on it, is blocked with cloud.syncing or is cancelled: that is
// what lets the check restore a game the library reports as starting
// (savebackup.RestoreBeforeLaunch). So a running check is joined before any
// gate is looked at, and a launch stops waiting without a block only while
// its check has proved nothing and is cancelled with it.
//
//wails:ignore
func (s *Service) SessionStarting(ctx context.Context, game library.Game) error {
	limit := s.launchTimeout
	if c := s.joinRunning(game.ID); c != nil {
		return s.awaitCheck(ctx, c, limit)
	}
	key := game.CanonicalGameID
	if key == "" || !account.ValidSaveGameID(key) {
		return nil
	}
	_, owner, err := s.signedIn(true)
	switch {
	case errors.Is(err, errDisabled), errors.Is(err, errNotSignedIn):
		return nil
	case err != nil:
		return checkError(err)
	}
	if duplicateKeys(s.lib.GetGames())[key] {
		return nil
	}
	st, err := s.recordedState(owner, key)
	if err != nil {
		return checkError(err)
	}
	switch {
	case st.Excluded:
		return nil
	case st.Conflict != nil:
		return blocked(ErrConflictUnresolved)
	}
	//nolint:contextcheck // the check belongs to the service and outlives this launch's wait, so it runs under s.ctx; ctx only ends the wait
	c, err := s.joinCheck(game, st)
	if err != nil {
		return err
	}
	return s.awaitCheck(ctx, c, limit)
}

func (s *Service) awaitCheck(ctx context.Context, c *launchCheck, limit time.Duration) error {
	timer := time.NewTimer(limit)
	defer timer.Stop()
	select {
	case <-c.done:
		s.leaveCheck(c, false)
		return c.err
	case <-ctx.Done():
		s.leaveCheck(c, true)
		return ctx.Err()
	case <-timer.C:
		return s.checkTimedOut(c, limit)
	}
}

// recordedState reads one game's record without installing a state: the
// launch check runs outside opMu, under which alone a state is installed.
func (s *Service) recordedState(owner, key string) (gameState, error) {
	s.mu.Lock()
	if s.loaded && s.owner == owner && s.loadErr == nil {
		st := s.state.Games[key]
		s.mu.Unlock()
		return st, nil
	}
	s.mu.Unlock()
	st, err := s.store.load(owner)
	if err != nil {
		return gameState{}, fmt.Errorf("%w: %w", errStateBroken, err)
	}
	return st.Games[key], nil
}

// joinRunning adds a launch to the running check of its game, if there is
// one that was not abandoned.
func (s *Service) joinRunning(gameID string) *launchCheck {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.joinLocked(gameID)
}

func (s *Service) joinLocked(gameID string) *launchCheck {
	c := s.launches[gameID]
	if c == nil || c.abandoned {
		return nil
	}
	c.waiters++
	return c
}

// joinCheck returns the running check of the game or starts one in a
// goroutine owned by the service, which outlives the launch's wait when a
// download has to finish.
func (s *Service) joinCheck(game library.Game, st gameState) (*launchCheck, error) {
	key := game.CanonicalGameID
	s.mu.Lock()
	defer s.mu.Unlock()
	if c := s.joinLocked(game.ID); c != nil {
		return c, nil
	}
	if s.fetching[key] {
		return nil, blocked(ErrSyncing)
	}
	if s.ctx == nil || s.closing {
		return nil, checkError(errNotStarted)
	}
	ctx, cancel := context.WithCancel(s.ctx)
	c := &launchCheck{gameID: game.ID, key: key, cancel: cancel, done: make(chan struct{}), waiters: 1}
	s.launches[game.ID] = c
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		defer cancel()
		err := s.runCheck(ctx, c, game, st)
		s.mu.Lock()
		if s.launches[game.ID] == c {
			delete(s.launches, game.ID)
		}
		unheard := c.waiters == 0
		s.mu.Unlock()
		var block *library.LaunchBlockedError
		if unheard && err != nil && ctx.Err() == nil && !errors.As(err, &block) {
			slog.Warn("cloud saves check finished after the launch stopped waiting", "game", game.ID, "error", err)
		}
		c.err = err
		close(c.done)
	}()
	return c, nil
}

// leaveCheck drops one waiter. A launch the player cancelled takes the check
// down with it only while the check has proved nothing and nobody else
// waits: a download once proved necessary was promised to go on.
func (s *Service) leaveCheck(c *launchCheck, cancelled bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c.waiters--
	if cancelled && c.waiters == 0 && c.known == nil {
		c.abandoned = true
		c.cancel()
	}
}

// checkTimedOut decides a launch whose check outlasted the limit. A download
// of the game, running or proved necessary, blocks and goes on in the
// background; so does a proved conflict on its way to the state file. A
// check that knows nothing yet is abandoned and the game starts.
func (s *Service) checkTimedOut(c *launchCheck, limit time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	c.waiters--
	select {
	case <-c.done:
		return c.err
	default:
	}
	switch {
	case s.fetching[c.key]:
		return blocked(ErrSyncing)
	case c.known != nil:
		return blocked(c.known)
	}
	c.abandoned = true
	c.cancel()
	return checkError(fmt.Errorf("no answer for %s within %v: %w", c.gameID, limit, context.DeadlineExceeded))
}

// claimFetch marks a download of key as running. An abandoned launch check
// never gets here: it is abandoned only while known is unset and is
// cancelled in the same step under mu, and runCheck sets known under mu
// before it takes opMu and checks ctx after taking it, before any download.
func (s *Service) claimFetch(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.fetching[key] = true
}

func (s *Service) releaseFetch(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.fetching, key)
}

// runCheck looks without opMu and acts under it. The listing and the hash
// run unlocked, so a pass or a transfer of another game, which holds opMu for
// its whole length, delays the launch by no more than the limit. A download
// or a new conflict changes the saves or the state, so it runs under opMu
// like a pass and decides again there from the state as it is then; a pass,
// SyncNow or ResolveConflict that got there first has settled the game.
func (s *Service) runCheck(ctx context.Context, c *launchCheck, g library.Game, st gameState) error {
	key := g.CanonicalGameID
	list, err := s.client.ListSaves(ctx)
	if err != nil {
		return checkFailed(ctx, err)
	}
	listed := &run{list: list, remote: make(map[string]account.Save, len(list.Games))}
	for _, save := range list.Games {
		listed.remote[save.GameID] = save
	}
	act, _, err := s.decideAtLaunch(ctx, g, st, listed)
	if err != nil {
		return checkFailed(ctx, err)
	}
	var known error
	switch act.kind {
	case actDownload:
		known = ErrSyncing
	case actConflict:
		known = ErrConflictUnresolved
	default:
		return nil
	}
	s.mu.Lock()
	c.known = known
	s.mu.Unlock()

	s.opMu.Lock()
	defer s.opMu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	r, err := s.identify(true)
	switch {
	case errors.Is(err, errDisabled), errors.Is(err, errNotSignedIn):
		return nil
	case err != nil:
		return checkFailed(ctx, err)
	case r.stateErr != nil:
		return checkFailed(ctx, r.stateErr)
	}
	r.list, r.remote = listed.list, listed.remote
	st = s.gameState(key)
	switch {
	case st.Excluded:
		return nil
	case st.Conflict != nil:
		return blocked(ErrConflictUnresolved)
	}
	act, t, err := s.decideAtLaunch(ctx, g, st, r)
	if err != nil {
		return checkFailed(ctx, err)
	}
	switch act.kind {
	case actDownload:
		out := s.download(ctx, r, g.ID, key, r.remote[key], c)
		if err := ctx.Err(); err != nil {
			return err
		}
		s.record(g.ID, out)
		if out.err != nil {
			// The local saves are the ones the player last synced: the game
			// may start on them, and the next pass records the conflict.
			return checkError(out.err)
		}
		return nil
	case actConflict:
		out := s.conflict(key, t, st, r.remote[key])
		s.record(g.ID, out)
		if out.state != StateConflict {
			// The conflict is real but not on disk: the game still must not
			// start, and the next pass records it.
			return blocked(fmt.Errorf("%w: %w", ErrConflictUnresolved, out.err))
		}
		return blocked(ErrConflictUnresolved)
	}
	return nil
}

func (s *Service) decideAtLaunch(ctx context.Context, g library.Game, st gameState, r *run) (action, tree, error) {
	root, missing, err := s.resolveRoot(ctx, g)
	if err != nil {
		return action{}, tree{}, err
	}
	var t tree
	if !missing {
		if t, err = s.treeOf(ctx, root); err != nil {
			return action{}, tree{}, err
		}
	}
	remote, hasRemote := r.remote[g.CanonicalGameID]
	return decide(modeAuto, t.digest, st, remote, hasRemote), t, nil
}

// checkFailed turns a failure before anything is known into a started game:
// a saves folder a pass would skip starts it quietly, anything else carries
// its cause to the library's log.
func checkFailed(ctx context.Context, err error) error {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	if skippable(err) {
		return nil
	}
	return checkError(err)
}

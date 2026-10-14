package cloudsaves

import (
	"errors"
	"fmt"

	"typhon/internal/account"
	"typhon/internal/library"
)

// gameForOp finds the game a player asked about and refuses one the cloud
// cannot address, loudly: such a request must not look like a quiet success.
func (s *Service) gameForOp(id string) (library.Game, error) {
	g, err := s.findGame(id)
	if err != nil {
		return library.Game{}, err
	}
	var bad error
	switch {
	case g.CanonicalGameID == "":
		bad = errNoCanonicalID
	case !account.ValidSaveGameID(g.CanonicalGameID):
		bad = errInvalidID
	}
	if bad != nil {
		s.record(g.ID, outcome{state: StateFailed, err: bad})
		return library.Game{}, bad
	}
	return g, nil
}

// gameForSync is gameForOp plus a refusal for a game that shares its catalog
// id with another live library entry.
func (s *Service) gameForSync(id string) (library.Game, error) {
	g, err := s.gameForOp(id)
	if err != nil {
		return library.Game{}, err
	}
	if duplicateKeys(s.lib.GetGames())[g.CanonicalGameID] {
		s.record(g.ID, fail(errDuplicate))
		return library.Game{}, errDuplicate
	}
	return g, nil
}

// SyncNow syncs one game right away. It also lifts an exclusion left by
// DeleteRemote: running it by hand is the player asking for the game back. A game that was skipped (running, saves
// folder not found) returns the reason as an error: the player asked for it.
func (s *Service) SyncNow(gameID string) error {
	ctx, end, err := s.begin()
	if err != nil {
		return err
	}
	defer end()
	s.opMu.Lock()
	defer s.opMu.Unlock()

	g, err := s.gameForSync(gameID)
	if err != nil {
		return err
	}
	r, err := s.prepare(ctx, true)
	if err != nil {
		return err
	}
	s.seedConflicts()
	if s.gameState(g.CanonicalGameID).Excluded {
		if err := s.commit(g.CanonicalGameID, func(st *gameState) { st.Excluded = false }); err != nil {
			return err
		}
	}
	out := s.syncGame(ctx, r, g, modeAuto)
	s.record(g.ID, out)
	return out.err
}

// ResolveConflict settles a recorded conflict: keepLocal overwrites the
// cloud copy with this device's saves, otherwise the cloud copy replaces the
// local saves after a safety snapshot of them.
func (s *Service) ResolveConflict(gameID string, keepLocal bool) error {
	ctx, end, err := s.begin()
	if err != nil {
		return err
	}
	defer end()
	s.opMu.Lock()
	defer s.opMu.Unlock()

	g, err := s.gameForSync(gameID)
	if err != nil {
		return err
	}
	r, err := s.prepare(ctx, true)
	if err != nil {
		return err
	}
	if s.gameState(g.CanonicalGameID).Conflict == nil {
		return errNoConflict
	}
	m := modeKeepRemote
	if keepLocal {
		m = modeKeepLocal
	}
	out := s.syncGame(ctx, r, g, m)
	s.record(g.ID, out)
	return out.err
}

// PullRemote takes the cloud copy without a recorded conflict: it is
// downloaded and restored through savebackup, which keeps a pre-restore copy of
// the current files.
func (s *Service) PullRemote(gameID string) error {
	ctx, end, err := s.begin()
	if err != nil {
		return err
	}
	defer end()
	s.opMu.Lock()
	defer s.opMu.Unlock()

	g, err := s.gameForSync(gameID)
	if err != nil {
		return err
	}
	r, err := s.prepare(ctx, true)
	if err != nil {
		return err
	}
	if _, ok := r.remote[g.CanonicalGameID]; !ok {
		return errNoRemote
	}
	out := s.syncGame(ctx, r, g, modeKeepRemote)
	s.record(g.ID, out)
	return out.err
}

// DeleteRemote removes the game's archive from the cloud and excludes the game
// from passes, otherwise the next one would upload the saves again. It works with the setting off: switching sync off must
// not leave the player unable to delete what is stored.
func (s *Service) DeleteRemote(gameID string) error {
	ctx, end, err := s.begin()
	if err != nil {
		return err
	}
	defer end()
	s.opMu.Lock()
	defer s.opMu.Unlock()

	g, err := s.gameForOp(gameID)
	if err != nil {
		return err
	}
	r, err := s.identify(false)
	if err != nil {
		return err
	}
	if r.stateErr != nil {
		return r.stateErr
	}
	if err := s.client.DeleteSave(ctx, g.CanonicalGameID); err != nil && !isNotFound(err) {
		return err
	}
	if err := s.commit(g.CanonicalGameID, func(st *gameState) { *st = gameState{Excluded: true} }); err != nil {
		return err
	}
	s.record(g.ID, outcome{state: StateSkipped, code: codeExcluded})
	return nil
}

// Forget deletes every archive the account holds and the recorded state. It
// is for switching the setting off together with removal; it keeps going past
// a failed delete and returns what failed, leaving those games recorded.
func (s *Service) Forget() error {
	ctx, end, err := s.begin()
	if err != nil {
		return err
	}
	defer end()
	s.opMu.Lock()
	defer s.opMu.Unlock()

	r, err := s.identify(false)
	if err != nil {
		return err
	}
	list, err := s.client.ListSaves(ctx)
	if err != nil {
		return err
	}
	var errs []error
	failed := map[string]bool{}
	for _, g := range list.Games {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := s.client.DeleteSave(ctx, g.GameID); err != nil && !isNotFound(err) {
			failed[g.GameID] = true
			errs = append(errs, fmt.Errorf("%s: %w", g.GameID, err))
		}
	}
	if r.stateErr != nil {
		errs = append(errs, r.stateErr)
		return errors.Join(errs...)
	}
	s.mu.Lock()
	keys := make([]string, 0, len(s.state.Games))
	for k := range s.state.Games {
		if !failed[k] {
			keys = append(keys, k)
		}
	}
	s.mu.Unlock()
	if err := s.drop(keys); err != nil {
		errs = append(errs, err)
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	s.mu.Lock()
	s.statuses = map[string]GameStatus{}
	s.usage.UsedBytes = 0
	s.usage.Games = nil
	s.mu.Unlock()
	s.record("", outcome{state: StateSkipped, code: codeRemoved})
	return nil
}

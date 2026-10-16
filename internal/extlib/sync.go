package extlib

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"typhon/internal/library"
)

func refKey(provider, appID string) string {
	return provider + "\x00" + appID
}

func (s *Service) syncLoop(ctx context.Context) {
	defer s.wg.Done()
	delay := firstSyncDelay
	for {
		tick, stop := s.timer(delay)
		select {
		case <-ctx.Done():
			stop()
			return
		case <-tick:
		}
		delay = syncInterval
		if err := s.syncTick(ctx); errors.Is(err, errors.ErrUnsupported) {
			return
		}
	}
}

func (s *Service) syncTick(ctx context.Context) error {
	s.mu.Lock()
	if s.closing || s.running {
		s.mu.Unlock()
		return nil
	}
	s.running = true
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.running = false
		s.mu.Unlock()
	}()

	err := s.syncOnce(ctx)
	if ctx.Err() != nil {
		return nil
	}
	s.noteSyncError(err)
	return err
}

func (s *Service) noteSyncError(err error) {
	if err == nil {
		s.lastSyncErr = ""
		return
	}
	if msg := err.Error(); msg != s.lastSyncErr {
		s.lastSyncErr = msg
		slog.Warn("sync external games", "error", err)
	}
}

func (s *Service) syncOnce(ctx context.Context) error {
	refs := s.library.ExternalRefs()
	if len(refs) == 0 {
		return nil
	}
	enabled := map[string]bool{}
	inLibrary := make(map[string]bool, len(refs))
	for _, r := range refs {
		enabled[r.Provider] = true
		inLibrary[refKey(r.Provider, r.AppID)] = true
	}
	scans, err := s.scan(ctx)
	if err != nil {
		return err
	}

	var failures []error
	present := map[string]map[string]bool{}
	var items []library.ExternalGame
	var fresh []Candidate
	for _, sc := range scans {
		if !enabled[sc.Provider] {
			continue
		}
		if sc.Err != nil {
			if !errors.Is(sc.Err, ErrNotInstalled) && !errors.Is(sc.Err, errors.ErrUnsupported) {
				failures = append(failures, fmt.Errorf("%s: %w", sc.Provider, sc.Err))
			}
			continue
		}
		seen := make(map[string]bool, len(sc.Games)+len(sc.Skipped))
		for _, c := range sc.Games {
			seen[c.AppID] = true
			switch {
			case inLibrary[refKey(c.Provider, c.AppID)]:
				items = append(items, externalOf(c, ""))
			case s.bound[refKey(c.Provider, c.AppID)] != "":
				items = append(items, externalOf(c, s.bound[refKey(c.Provider, c.AppID)]))
			default:
				fresh = append(fresh, c)
			}
		}
		complete := true
		for _, sk := range sc.Skipped {
			if sk.AppID == "" {
				complete = false
				continue
			}
			seen[sk.AppID] = true
		}
		if complete {
			present[sc.Provider] = seen
		}
	}

	if len(fresh) > 0 {
		bound, err := s.bind(ctx, fresh)
		if err != nil {
			failures = append(failures, err)
		}
		for _, it := range bound {
			s.bound[refKey(it.Provider, it.AppID)] = it.CanonicalGameID
			items = append(items, it)
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	applied, err := s.library.SyncExternal(ctx, items, present)
	if err != nil {
		return errors.Join(append(failures, fmt.Errorf("sync external games: %w", err))...)
	}
	s.refreshAdded(ctx, applied, items)
	return errors.Join(failures...)
}

func (s *Service) refreshAdded(ctx context.Context, applied library.ExternalSummary, items []library.ExternalGame) {
	canonical := make(map[string]library.ExternalGame, len(items))
	for _, it := range items {
		canonical[refKey(it.Provider, it.AppID)] = it
	}
	for _, o := range applied.Outcomes {
		if !o.Added {
			continue
		}
		it := canonical[refKey(o.Provider, o.AppID)]
		if it.CanonicalGameID == "" || ctx.Err() != nil {
			continue
		}
		if _, err := s.metadata.EnsureFresh(it.CanonicalGameID); err != nil {
			slog.Warn("refresh metadata for synced game", "title", strings.TrimSpace(it.Title), "error", err)
		}
	}
}

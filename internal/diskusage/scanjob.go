package diskusage

import (
	"context"
	"errors"
)

// errWalkStale cancels a walk a delete made out of date; whoever waits on it
// waits on the next walk instead.
var errWalkStale = errors.New("a delete changed the disk under the walk")

// walk is the one disk walk in progress. Every Scan request that arrives while
// it runs waits on it rather than starting its own: a walk takes long, and
// requests come from the same page entered again or refreshed, so the answer
// of the running walk is what they ask for. The walk runs on the service's
// context, not a caller's, and stops once no request waits on it any more.
type walk struct {
	done    chan struct{}
	cancel  context.CancelCauseFunc
	waiters int
	report  Report
	err     error
}

// Scan returns a report of the disk usage and keeps it for Cached. A request
// that arrives during a walk joins it; one whose context ends leaves it, and
// the walk stops when nobody waits on it. A delete does not wait for a walk:
// it stops it, and the requests waiting on it wait on a walk of the disk the
// delete left.
func (s *Service) Scan(ctx context.Context) (Report, error) {
	if err := ctx.Err(); err != nil {
		return Report{}, err
	}
	for {
		w, err := s.joinWalk() //nolint:contextcheck // the walk lives on the service's context: one caller leaving must not stop it for the others
		if err != nil {
			return Report{}, err
		}
		select {
		case <-w.done:
			s.leaveWalk(w)
			if errors.Is(w.err, errWalkStale) {
				if err := ctx.Err(); err != nil {
					return Report{}, err
				}
				continue
			}
			if w.err != nil {
				return Report{}, w.err
			}
			return w.report.clone(), nil
		case <-ctx.Done():
			s.leaveWalk(w)
			return Report{}, ctx.Err()
		}
	}
}

// CancelScan stops the running walk, if any; the requests waiting on it get
// diskusage.scan_cancelled. With no walk running it does nothing, so a page
// may call it whenever it is left.
func (s *Service) CancelScan() {
	s.stopWalk(errScanCancelled)
}

func (s *Service) joinWalk() (*walk, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ctx == nil || s.closing {
		return nil, errNotStarted
	}
	w := s.walk
	if w == nil {
		ctx, cancel := context.WithCancelCause(s.ctx)
		w = &walk{done: make(chan struct{}), cancel: cancel}
		s.walk = w
		s.wg.Add(1)
		go s.runWalk(ctx, w)
	}
	w.waiters++
	return w, nil
}

func (s *Service) leaveWalk(w *walk) {
	s.mu.Lock()
	defer s.mu.Unlock()
	w.waiters--
	if w.waiters == 0 && s.walk == w {
		s.walk = nil
		w.cancel(context.Canceled)
	}
}

// stopWalk cancels the running walk with cause and forgets it, so the next
// request starts a new one. The cancelled walk never publishes: it checks its
// context under the same lock before it does.
func (s *Service) stopWalk(cause error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.walk != nil {
		s.walk.cancel(cause)
		s.walk = nil
	}
}

func (s *Service) runWalk(ctx context.Context, w *walk) {
	defer s.wg.Done()
	defer close(w.done)
	defer w.cancel(context.Canceled)
	// A delete under way finishes first, so the walk measures the disk it
	// leaves; a delete that starts later stops the walk (stopWalk).
	s.opMu.Lock()
	s.opMu.Unlock() //nolint:staticcheck // SA2001: an empty critical section waits for the running delete

	sc, err := s.walkDisk(ctx)
	s.mu.Lock()
	defer s.mu.Unlock()
	if ctx.Err() != nil {
		err = context.Cause(ctx)
	}
	if s.walk == w {
		s.walk = nil
	}
	if err != nil {
		w.err = err
		return
	}
	stored := sc.report.clone()
	s.report = &stored
	s.entries = sc.entries
	s.sets = sc.sets
	w.report = sc.report
}

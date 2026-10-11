package account

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"typhon/internal/legal"
)

const (
	termsRetryMin = time.Minute
	termsRetryMax = 30 * time.Minute
)

type termsOutcome int

const (
	termsConfirmed termsOutcome = iota
	termsGone
	termsFailed
	termsRejected
)

type termsRun struct {
	token     string
	userID    string
	cancel    context.CancelFunc
	known     *TermsAcceptance
	haveKnown bool
}

func newTermsTimer(d time.Duration) (<-chan time.Time, func()) {
	t := time.NewTimer(d)
	return t.C, func() { t.Stop() }
}

//wails:ignore
func (s *Service) SetAcceptedTerms(accepted func() string) {
	s.mu.Lock()
	s.accepted = accepted
	s.mu.Unlock()
}

func (s *Service) acceptedTerms() string {
	s.mu.Lock()
	accepted := s.accepted
	s.mu.Unlock()
	if accepted == nil {
		return ""
	}
	return accepted()
}

func (s *Service) registerTermsVersion() string {
	if s.acceptedTerms() != legal.Version {
		return ""
	}
	return legal.Version
}

//wails:ignore
func (s *Service) SyncTerms() {
	if s.acceptedTerms() != legal.Version {
		return
	}
	s.mu.Lock()
	token, id := s.syncToken, s.syncUserID
	skip := token == "" || id == "" || s.termsConfirmedID == id
	s.mu.Unlock()
	if skip {
		return
	}
	s.startTermsSync(token, id, nil, false)
}

func (s *Service) syncTermsOf(token string, user CurrentUser) {
	if s.acceptedTerms() != legal.Version {
		return
	}
	s.startTermsSync(token, user.ID, user.TermsAccepted, true)
}

func (s *Service) startTermsSync(token, userID string, known *TermsAcceptance, haveKnown bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ctx == nil || s.ctx.Err() != nil {
		return
	}
	if current := s.termsRun; current != nil {
		// A settings change (haveKnown is false) joins the running retry; a sign-in or a
		// restored session carries a fresh server reading and starts over at once.
		if !haveKnown && current.token == token && current.userID == userID {
			return
		}
		current.cancel()
	}
	ctx, cancel := context.WithCancel(s.ctx)
	run := &termsRun{token: token, userID: userID, cancel: cancel, known: known, haveKnown: haveKnown}
	s.termsRun = run
	s.wg.Add(1)

	go func() {
		defer s.wg.Done()
		defer s.releaseTermsRun(run)
		s.syncTermsUntilDone(ctx, run)
	}()
}

func (s *Service) releaseTermsRun(run *termsRun) {
	run.cancel()
	s.mu.Lock()
	if s.termsRun == run {
		s.termsRun = nil
	}
	s.mu.Unlock()
}

func (s *Service) syncTermsUntilDone(ctx context.Context, run *termsRun) {
	delay := termsRetryMin
	for {
		wait := delay
		switch s.pushTerms(ctx, run) {
		case termsConfirmed, termsGone:
			return
		case termsRejected:
			// The server answered and refused (terms_required while the backend still
			// lacks this revision, no endpoint on an old backend): asking again every
			// minute cannot help, only a backend deploy can, so check at the ceiling.
			wait, delay = termsRetryMax, termsRetryMax
		case termsFailed:
			delay = min(delay*2, termsRetryMax)
		}

		after, stop := s.termsTimer(wait)
		select {
		case <-ctx.Done():
			stop()
			return
		case <-after:
			stop()
		}
	}
}

func (s *Service) pushTerms(parent context.Context, run *termsRun) termsOutcome {
	if parent.Err() != nil || s.acceptedTerms() != legal.Version {
		return termsGone
	}
	current, err := s.token()
	switch {
	case errors.Is(err, ErrNoCredential):
		slog.Debug("stop terms acceptance sync: the session is gone")
		return termsGone
	case err != nil:
		slog.Warn("read the session for terms acceptance sync", "error", err)
		return termsFailed
	case current != run.token || s.SyncAccountID(run.token) != run.userID:
		slog.Debug("stop terms acceptance sync: the session changed")
		return termsGone
	}

	ctx, cancel := context.WithTimeout(parent, requestTimeout)
	defer cancel()
	client := *s.client
	client.token = func() (string, error) { return run.token, nil }

	if !run.haveKnown {
		user, err := client.Me(ctx)
		if err != nil {
			if parent.Err() != nil {
				return termsGone
			}
			slog.Warn("read terms acceptance from the server", "error", err)
			return termsOutcomeOf(err)
		}
		run.known, run.haveKnown = user.TermsAccepted, true
	}
	if run.known != nil && run.known.Version == legal.Version {
		s.confirmTerms(run.userID)
		return termsConfirmed
	}

	user, err := client.AcceptTerms(ctx, legal.Version)
	if err != nil {
		if parent.Err() != nil {
			return termsGone
		}
		slog.Warn("record terms acceptance on the server", "version", legal.Version, "error", err)
		return termsOutcomeOf(err)
	}
	if user.TermsAccepted == nil || user.TermsAccepted.Version != legal.Version {
		slog.Warn("server did not record the terms acceptance", "version", legal.Version)
		return termsFailed
	}
	s.confirmTerms(run.userID)
	return termsConfirmed
}

func termsOutcomeOf(err error) termsOutcome {
	var apiErr *Error
	if errors.As(err, &apiErr) && apiErr.Status >= http.StatusBadRequest && apiErr.Status < http.StatusInternalServerError &&
		apiErr.Status != http.StatusRequestTimeout && apiErr.Status != http.StatusTooManyRequests {
		return termsRejected
	}
	return termsFailed
}

func (s *Service) confirmTerms(userID string) {
	s.mu.Lock()
	s.termsConfirmedID = userID
	s.mu.Unlock()
}

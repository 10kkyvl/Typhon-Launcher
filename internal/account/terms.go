package account

import (
	"context"
	"log/slog"

	"typhon/internal/legal"
)

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
	skip := token == "" || id == "" || s.termsConfirmedID == id || s.termsTriedID == id
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
	if s.ctx == nil || s.ctx.Err() != nil {
		s.mu.Unlock()
		return
	}
	ctx := s.ctx
	s.termsTriedID = userID
	s.wg.Add(1)
	s.mu.Unlock()

	go func() {
		defer s.wg.Done()
		s.pushTerms(ctx, token, userID, known, haveKnown)
	}()
}

func (s *Service) pushTerms(parent context.Context, token, userID string, known *TermsAcceptance, haveKnown bool) {
	ctx, cancel := context.WithTimeout(parent, requestTimeout)
	defer cancel()

	current, err := s.token()
	if err != nil || current != token {
		slog.Debug("skip terms acceptance sync: the session is gone", "error", err)
		return
	}
	client := *s.client
	client.token = func() (string, error) { return token, nil }

	if !haveKnown {
		user, err := client.Me(ctx)
		if err != nil {
			slog.Warn("read terms acceptance from the server", "error", err)
			return
		}
		known = user.TermsAccepted
	}
	if known != nil && known.Version == legal.Version {
		s.confirmTerms(userID)
		return
	}

	user, err := client.AcceptTerms(ctx, legal.Version)
	if err != nil {
		slog.Warn("record terms acceptance on the server", "version", legal.Version, "error", err)
		return
	}
	if user.TermsAccepted == nil || user.TermsAccepted.Version != legal.Version {
		slog.Warn("server did not record the terms acceptance", "version", legal.Version)
		return
	}
	s.confirmTerms(userID)
}

func (s *Service) confirmTerms(userID string) {
	s.mu.Lock()
	s.termsConfirmedID = userID
	s.mu.Unlock()
}

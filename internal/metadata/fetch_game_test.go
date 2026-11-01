package metadata

import (
	"context"
	"errors"
	"testing"
	"time"
)

type countingProvider struct {
	calls int
	err   error
}

func (p *countingProvider) Name() string { return "counting" }

func (p *countingProvider) Search(context.Context, string, int) ([]Candidate, error) {
	return nil, nil
}

func (p *countingProvider) Get(context.Context, string) (GameMetadata, error) {
	p.calls++
	return GameMetadata{ProviderID: "7", Title: "Seven"}, p.err
}

func TestFetchGameWithoutProvider(t *testing.T) {
	s := &Service{budget: newBudget(time.Now)}
	if _, err := s.FetchGame(t.Context(), "7"); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("error = %v", err)
	}
}

func TestFetchGameRespectsTheRateLimitPause(t *testing.T) {
	provider := &countingProvider{}
	s := &Service{budget: newBudget(time.Now), provider: provider}
	s.budget.penalize(time.Minute)
	_, err := s.FetchGame(t.Context(), "7")
	var limit *RateLimitError
	if !errors.As(err, &limit) || limit.RetryAfter <= 0 {
		t.Fatalf("error = %v, want a rate limit with a wait", err)
	}
	if provider.calls != 0 {
		t.Fatalf("provider called %d times during the pause", provider.calls)
	}
}

func TestFetchGameStartsThePauseAfterA429(t *testing.T) {
	provider := &countingProvider{err: &RateLimitError{RetryAfter: time.Minute}}
	s := &Service{budget: newBudget(time.Now), provider: provider}
	if _, err := s.FetchGame(t.Context(), "7"); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("error = %v", err)
	}
	if _, _, penalties, pause := s.budget.stats(); penalties != 1 || pause <= 0 {
		t.Fatalf("penalties=%d pause=%v", penalties, pause)
	}
}

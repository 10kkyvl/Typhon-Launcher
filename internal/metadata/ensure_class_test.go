package metadata

import (
	"context"
	"testing"
	"time"

	"typhon/internal/catalog"
)

type classProbe struct {
	fakeProvider
	svc  *Service
	user int
	back int
}

func (p *classProbe) Get(ctx context.Context, providerID string) (GameMetadata, error) {
	p.mu.Lock()
	p.user = len(p.svc.budget.user)
	p.back = len(p.svc.budget.back)
	p.mu.Unlock()
	return p.fakeProvider.Get(ctx, providerID)
}

func TestEnsureFreshPicksCallClass(t *testing.T) {
	old := time.Now().Add(-defaultTTL - time.Hour)
	recent := time.Now().Add(-time.Hour)
	cases := []struct {
		name     string
		game     catalog.Game
		wantUser bool
	}{
		{
			name: "complete metadata past the ttl yields to the user",
			game: catalog.Game{Title: "Prey", ExternalIDs: catalog.ExternalIDs{IGDB: "2657"}, MetadataUpdatedAt: &old, MetadataLanguage: "en"},
		},
		{
			name: "complete metadata in another language yields to the user",
			game: catalog.Game{Title: "Prey", ExternalIDs: catalog.ExternalIDs{IGDB: "2657"}, MetadataUpdatedAt: &recent, MetadataLanguage: "ru"},
		},
		{
			name:     "no metadata yet is a user request",
			game:     catalog.Game{Title: "Prey", ExternalIDs: catalog.ExternalIDs{IGDB: "2657"}},
			wantUser: true,
		},
		{
			name:     "partial metadata is a user request",
			game:     catalog.Game{Title: "Prey", ExternalIDs: catalog.ExternalIDs{IGDB: "2657"}, MetadataUpdatedAt: &old, MetadataLanguage: "en", MetadataPartial: true},
			wantUser: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := imageServer(t)
			probe := &classProbe{fakeProvider: fakeProvider{meta: map[string]GameMetadata{"2657": fullMetadata(srv.URL)}}}
			svc, cat, _ := newTestService(t, probe)
			probe.svc = svc
			game := addGame(t, cat, tc.game)

			started, err := svc.EnsureFresh(game.ID)
			if err != nil {
				t.Fatalf("ensure fresh: %v", err)
			}
			if !started {
				t.Fatal("refresh did not start")
			}
			svc.wg.Wait()

			probe.mu.Lock()
			user, back := probe.user, probe.back
			probe.mu.Unlock()
			if tc.wantUser && (user != 1 || back != 0) {
				t.Fatalf("slots during the provider call: user=%d background=%d, want a user-class call", user, back)
			}
			if !tc.wantUser && (user != 0 || back != 1) {
				t.Fatalf("slots during the provider call: user=%d background=%d, want a background-class call", user, back)
			}
		})
	}
}

func TestEnsureFreshYieldedBackgroundRefreshRecordsNoAttempt(t *testing.T) {
	srv := imageServer(t)
	provider := &fakeProvider{meta: map[string]GameMetadata{"2657": fullMetadata(srv.URL)}}
	svc, cat, _ := newTestService(t, provider)
	old := time.Now().Add(-defaultTTL - time.Hour)
	game := addGame(t, cat, catalog.Game{
		Title:             "Prey",
		ExternalIDs:       catalog.ExternalIDs{IGDB: "2657"},
		MetadataUpdatedAt: &old,
		MetadataLanguage:  "en",
	})

	svc.budget.mu.Lock()
	svc.budget.pausedUntil = time.Now().Add(time.Hour)
	svc.budget.mu.Unlock()

	started, err := svc.EnsureFresh(game.ID)
	if err != nil {
		t.Fatalf("ensure fresh: %v", err)
	}
	if !started {
		t.Fatal("refresh did not start")
	}
	svc.wg.Wait()

	if _, gets := provider.counts(); gets != 0 {
		t.Fatalf("provider called %d times while the budget was busy", gets)
	}
	if rec, ok := svc.attempts.state(game.ID); ok {
		t.Fatalf("a yielded background refresh recorded an attempt: %+v", rec)
	}

	resetBudget(svc)
	unthrottle(svc)
	started, err = svc.EnsureFresh(game.ID)
	if err != nil {
		t.Fatalf("second ensure fresh: %v", err)
	}
	if !started {
		t.Fatal("the next open did not retry the refresh")
	}
	svc.wg.Wait()
	updated, err := cat.GetGame(game.ID)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if updated.MetadataUpdatedAt == nil || !updated.MetadataUpdatedAt.After(old) {
		t.Fatalf("timestamp not advanced: %v", updated.MetadataUpdatedAt)
	}
}

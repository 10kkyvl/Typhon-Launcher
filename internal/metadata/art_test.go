package metadata

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"typhon/internal/catalog"
)

func TestEnsureArtStoresCoverWithoutScreenshots(t *testing.T) {
	srv := imageServer(t)
	provider := &fakeProvider{meta: map[string]GameMetadata{"2657": fullMetadata(srv.URL)}}
	svc, cat, _ := newTestService(t, provider)
	game := addGame(t, cat, catalog.Game{Title: "Prey", ExternalIDs: catalog.ExternalIDs{IGDB: "2657"}})

	accepted, err := svc.EnsureArt([]string{game.ID})
	if err != nil {
		t.Fatalf("ensure art: %v", err)
	}
	if len(accepted) != 1 || accepted[0] != game.ID {
		t.Fatalf("accepted = %v, want [%s]", accepted, game.ID)
	}
	svc.wg.Wait()

	assets := svc.store.list(game.ID)
	if len(assets) != 1 || assets[0].Type != AssetCover {
		t.Fatalf("stored assets = %+v, want a single cover", assets)
	}

	updated, err := cat.GetGame(game.ID)
	if err != nil {
		t.Fatalf("reload game: %v", err)
	}
	if !updated.MetadataPartial {
		t.Fatal("game not marked as partially resolved")
	}
	if updated.CoverAssetID == "" || updated.Developer != "Arkane Studios" || updated.Summary == "" {
		t.Fatalf("text metadata not applied: %+v", updated)
	}
	if updated.HeroAssetID != "" {
		t.Fatalf("hero = %q, want empty without screenshots", updated.HeroAssetID)
	}
}

func TestEnsureFreshCompletesPartialMetadata(t *testing.T) {
	srv := imageServer(t)
	provider := &fakeProvider{meta: map[string]GameMetadata{"2657": fullMetadata(srv.URL)}}
	svc, cat, _ := newTestService(t, provider)
	game := addGame(t, cat, catalog.Game{Title: "Prey", ExternalIDs: catalog.ExternalIDs{IGDB: "2657"}})

	if _, err := svc.EnsureArt([]string{game.ID}); err != nil {
		t.Fatalf("ensure art: %v", err)
	}
	svc.wg.Wait()

	started, err := svc.EnsureFresh(game.ID)
	if err != nil {
		t.Fatalf("ensure fresh: %v", err)
	}
	if !started {
		t.Fatal("partial metadata was not completed")
	}
	svc.wg.Wait()

	if got := len(svc.screenshots(game.ID)); got != maxScreenshots {
		t.Fatalf("screenshots = %d, want %d", got, maxScreenshots)
	}
	updated, err := cat.GetGame(game.ID)
	if err != nil {
		t.Fatalf("reload game: %v", err)
	}
	if updated.MetadataPartial {
		t.Fatal("partial flag not cleared after a full refresh")
	}
	if updated.HeroAssetID == "" {
		t.Fatal("hero not selected after a full refresh")
	}
}

func TestEnsureArtSkipsGamesThatAlreadyHaveArt(t *testing.T) {
	srv := imageServer(t)
	provider := &fakeProvider{meta: map[string]GameMetadata{"2657": fullMetadata(srv.URL)}}
	svc, cat, _ := newTestService(t, provider)
	game := addGame(t, cat, catalog.Game{Title: "Prey"})

	if _, err := svc.ApplyMatch(game.ID, "2657"); err != nil {
		t.Fatalf("apply match: %v", err)
	}
	_, gets := provider.counts()

	accepted, err := svc.EnsureArt([]string{game.ID})
	if err != nil {
		t.Fatalf("ensure art: %v", err)
	}
	svc.wg.Wait()
	if len(accepted) != 1 {
		t.Fatalf("accepted = %v, want the game to be accepted as done", accepted)
	}
	if _, after := provider.counts(); after != gets {
		t.Fatalf("provider called %d times, want %d", after, gets)
	}
}

func TestEnsureArtInputHandling(t *testing.T) {
	srv := imageServer(t)
	provider := &fakeProvider{meta: map[string]GameMetadata{"2657": fullMetadata(srv.URL)}}
	svc, cat, _ := newTestService(t, provider)
	known := addGame(t, cat, catalog.Game{Title: "Prey", ExternalIDs: catalog.ExternalIDs{IGDB: "2657"}})

	cases := []struct {
		name  string
		ids   []string
		count int
	}{
		{"no ids", nil, 0},
		{"empty id skipped", []string{"", "   "}, 0},
		{"unknown game accepted", []string{"missing"}, 1},
		{"known game accepted", []string{known.ID}, 1},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			accepted, err := svc.EnsureArt(tc.ids)
			if err != nil {
				t.Fatalf("ensure art: %v", err)
			}
			if len(accepted) != tc.count {
				t.Fatalf("accepted = %v, want %d ids", accepted, tc.count)
			}
		})
	}
	svc.wg.Wait()
}

func TestEnsureArtCapsTheBatch(t *testing.T) {
	srv := imageServer(t)
	provider := &fakeProvider{meta: map[string]GameMetadata{"2657": fullMetadata(srv.URL)}}
	svc, _, _ := newTestService(t, provider)

	ids := make([]string, 0, maxArtBatch+8)
	for i := range maxArtBatch + 8 {
		ids = append(ids, fmt.Sprintf("unknown-%d", i))
	}

	accepted, err := svc.EnsureArt(ids)
	if err != nil {
		t.Fatalf("ensure art: %v", err)
	}
	if len(accepted) != maxArtBatch {
		t.Fatalf("accepted = %d ids, want the batch cap %d", len(accepted), maxArtBatch)
	}
}

func TestEnsureArtDefersWhenTheQueueIsFull(t *testing.T) {
	srv := imageServer(t)
	provider := &fakeProvider{meta: map[string]GameMetadata{"2657": fullMetadata(srv.URL)}}
	svc, cat, _ := newTestService(t, provider)

	ids := make([]string, 0, maxArtQueue+4)
	for i := range maxArtQueue + 4 {
		game := addGame(t, cat, catalog.Game{
			Title:       fmt.Sprintf("Game %02d", i),
			ExternalIDs: catalog.ExternalIDs{IGDB: "2657"},
		})
		ids = append(ids, game.ID)
	}

	svc.mu.Lock()
	for i := range maxArtQueue {
		svc.refreshing[fmt.Sprintf("busy-%d", i)] = true
	}
	svc.mu.Unlock()

	accepted, err := svc.EnsureArt(ids)
	if err != nil {
		t.Fatalf("ensure art: %v", err)
	}
	if len(accepted) != 0 {
		t.Fatalf("accepted = %d ids while the queue is full", len(accepted))
	}

	svc.mu.Lock()
	for i := range maxArtQueue {
		delete(svc.refreshing, fmt.Sprintf("busy-%d", i))
	}
	svc.mu.Unlock()

	accepted, err = svc.EnsureArt(ids)
	if err != nil {
		t.Fatalf("ensure art: %v", err)
	}
	if len(accepted) != maxArtBatch {
		t.Fatalf("accepted = %d ids, want a full batch of %d", len(accepted), maxArtBatch)
	}
	svc.wg.Wait()
}

func TestEnsureArtRefusesAfterShutdown(t *testing.T) {
	srv := imageServer(t)
	provider := &fakeProvider{meta: map[string]GameMetadata{"2657": fullMetadata(srv.URL)}}
	svc, cat, _ := newTestService(t, provider)
	game := addGame(t, cat, catalog.Game{Title: "Prey", ExternalIDs: catalog.ExternalIDs{IGDB: "2657"}})

	svc.mu.Lock()
	svc.closing = true
	svc.mu.Unlock()

	if _, err := svc.EnsureArt([]string{game.ID}); err == nil {
		t.Fatal("ensure art accepted work while shutting down")
	}

	svc.mu.Lock()
	svc.closing = false
	svc.mu.Unlock()
}

func TestEnsureArtGivesUpOnALongCooldownAndStaysRetryable(t *testing.T) {
	srv := imageServer(t)
	provider := &fakeProvider{meta: map[string]GameMetadata{"2657": fullMetadata(srv.URL)}}
	svc, cat, _ := newTestService(t, provider)
	game := addGame(t, cat, catalog.Game{Title: "Prey", ExternalIDs: catalog.ExternalIDs{IGDB: "2657"}})

	svc.budget.penalize(time.Hour)

	accepted, err := svc.EnsureArt([]string{game.ID})
	if err != nil {
		t.Fatalf("ensure art: %v", err)
	}
	if len(accepted) != 1 {
		t.Fatalf("accepted = %v, want the game taken for a background refresh", accepted)
	}
	svc.wg.Wait()
	if _, gets := provider.counts(); gets != 0 {
		t.Fatalf("provider called %d times during a cooldown", gets)
	}

	if !svc.attempts.due(game.ID, false) {
		t.Fatal("a game skipped because of a cooldown must stay retryable")
	}
	resumeBudget(svc)

	if _, err := svc.EnsureArt([]string{game.ID}); err != nil {
		t.Fatalf("ensure art after the cooldown: %v", err)
	}
	svc.wg.Wait()

	updated, err := cat.GetGame(game.ID)
	if err != nil {
		t.Fatalf("reload game: %v", err)
	}
	if updated.CoverAssetID == "" {
		t.Fatal("cover not stored once the cooldown was over")
	}
}

type limitedProvider struct {
	*fakeProvider
	limits  sync.Mutex
	fail    int
	waitFor time.Duration
}

func (p *limitedProvider) Get(ctx context.Context, providerID string) (GameMetadata, error) {
	p.limits.Lock()
	limited := p.fail > 0
	if limited {
		p.fail--
	}
	p.limits.Unlock()
	if !limited {
		return p.fakeProvider.Get(ctx, providerID)
	}

	p.mu.Lock()
	p.gets++
	p.mu.Unlock()
	return GameMetadata{}, &RateLimitError{RetryAfter: p.waitFor}
}

func TestEnsureArtRetriesAfterTheCooldown(t *testing.T) {
	srv := imageServer(t)
	provider := &limitedProvider{
		fakeProvider: &fakeProvider{meta: map[string]GameMetadata{"2657": fullMetadata(srv.URL)}},
		fail:         1,
		waitFor:      10 * time.Millisecond,
	}
	svc, cat, _ := newTestService(t, provider)
	game := addGame(t, cat, catalog.Game{Title: "Prey", ExternalIDs: catalog.ExternalIDs{IGDB: "2657"}})

	if _, err := svc.EnsureArt([]string{game.ID}); err != nil {
		t.Fatalf("ensure art: %v", err)
	}
	svc.wg.Wait()

	updated, err := cat.GetGame(game.ID)
	if err != nil {
		t.Fatalf("reload game: %v", err)
	}
	if updated.CoverAssetID == "" {
		t.Fatal("cover not stored after the provider cooldown expired")
	}
}

func resumeBudget(svc *Service) {
	svc.budget.mu.Lock()
	svc.budget.pausedUntil = time.Time{}
	svc.budget.mu.Unlock()
}

func TestRateLimitShrinksTheRequestRateAndSuccessRestoresIt(t *testing.T) {
	svc, _, _ := newTestService(t, &fakeProvider{})
	resetBudget(svc)
	limit := &RateLimitError{RetryAfter: time.Second}

	previous := startRate
	for step := range 6 {
		if err := svc.notePenalty(limit); err == nil {
			t.Fatal("note penalty swallowed the error")
		}
		rate, _, penalties, _ := svc.budget.stats()
		if rate > previous {
			t.Fatalf("step %d: rate rose from %v to %v under a rate limit", step, previous, rate)
		}
		if rate < minRate {
			t.Fatalf("step %d: rate = %v, want it clamped at %v", step, rate, minRate)
		}
		if penalties != step+1 {
			t.Fatalf("step %d: penalties = %d", step, penalties)
		}
		previous = rate
	}
	if previous != minRate {
		t.Fatalf("rate after repeated limits = %v, want the floor %v", previous, minRate)
	}

	svc.budget.reward()
	after, _, _, _ := svc.budget.stats()
	if after <= previous {
		t.Fatalf("rate after a success = %v, want it above %v", after, previous)
	}
}

func TestRateLimitKeepsTheGameRetryable(t *testing.T) {
	provider := &fakeProvider{getErr: &RateLimitError{RetryAfter: time.Millisecond}}
	svc, cat, _ := newTestService(t, provider)
	game := addGame(t, cat, catalog.Game{Title: "Prey", ExternalIDs: catalog.ExternalIDs{IGDB: "2657"}})

	if _, err := svc.EnsureArt([]string{game.ID}); err != nil {
		t.Fatalf("ensure art: %v", err)
	}
	svc.wg.Wait()

	if !svc.attempts.due(game.ID, false) {
		t.Fatal("a rate limit must not count as a failed attempt for the game")
	}
}

type batchProvider struct {
	*fakeProvider
	batch      sync.Mutex
	calls      int
	lastTitles []string
	resolve    map[string]GameMetadata
	resolveErr error
}

func (p *batchProvider) Resolve(_ context.Context, titles []string) ([]Resolved, error) {
	p.batch.Lock()
	defer p.batch.Unlock()
	p.calls++
	p.lastTitles = append([]string(nil), titles...)
	if p.resolveErr != nil {
		return nil, p.resolveErr
	}
	out := make([]Resolved, 0, len(titles))
	for _, title := range titles {
		meta, ok := p.resolve[title]
		if !ok {
			continue
		}
		out = append(out, Resolved{Title: title, Meta: meta})
	}
	return out, nil
}

func (p *batchProvider) counts() (int, []string) {
	p.batch.Lock()
	defer p.batch.Unlock()
	return p.calls, append([]string(nil), p.lastTitles...)
}

func newBatchProvider(t *testing.T, base string, titles ...string) *batchProvider {
	t.Helper()
	resolve := make(map[string]GameMetadata, len(titles))
	for _, title := range titles {
		meta := fullMetadata(base)
		meta.Title = title
		resolve[title] = meta
	}
	return &batchProvider{
		fakeProvider: &fakeProvider{meta: map[string]GameMetadata{"2657": fullMetadata(base)}},
		resolve:      resolve,
	}
}

func TestEnsureArtResolvesTheWholeBatchInOneCall(t *testing.T) {
	srv := imageServer(t)
	provider := newBatchProvider(t, srv.URL, "Prey", "Dishonored")
	svc, cat, _ := newTestService(t, provider)

	first := addGame(t, cat, catalog.Game{Title: "Prey"})
	second := addGame(t, cat, catalog.Game{Title: "Dishonored"})

	accepted, err := svc.EnsureArt([]string{first.ID, second.ID})
	if err != nil {
		t.Fatalf("ensure art: %v", err)
	}
	if len(accepted) != 2 {
		t.Fatalf("accepted = %v, want both games", accepted)
	}
	svc.wg.Wait()

	calls, titles := provider.counts()
	if calls != 1 {
		t.Fatalf("resolve calls = %d, want one call for the whole batch", calls)
	}
	if len(titles) != 2 {
		t.Fatalf("resolve got %v, want both titles", titles)
	}
	if searches, gets := provider.fakeProvider.counts(); searches != 0 || gets != 0 {
		t.Fatalf("per-game lookups = %d searches, %d gets, want none", searches, gets)
	}

	for _, game := range []catalog.Game{first, second} {
		stored, err := cat.GetGame(game.ID)
		if err != nil {
			t.Fatalf("reload %s: %v", game.Title, err)
		}
		if stored.CoverAssetID == "" {
			t.Fatalf("%s has no cover after the batch", game.Title)
		}
		if !stored.MetadataPartial {
			t.Fatalf("%s is not marked as partially resolved", game.Title)
		}
		if shots := len(svc.screenshots(game.ID)); shots != 0 {
			t.Fatalf("%s stored %d screenshots, want none in art mode", game.Title, shots)
		}
	}
}

func TestEnsureArtFallsBackToSearchForTitlesTheBatchMissed(t *testing.T) {
	srv := imageServer(t)
	provider := newBatchProvider(t, srv.URL, "Prey")
	provider.candidates = []Candidate{{ProviderID: "2657", Title: "Dishonored"}}
	svc, cat, _ := newTestService(t, provider)

	resolved := addGame(t, cat, catalog.Game{Title: "Prey"})
	missed := addGame(t, cat, catalog.Game{Title: "Dishonored"})

	if _, err := svc.EnsureArt([]string{resolved.ID, missed.ID}); err != nil {
		t.Fatalf("ensure art: %v", err)
	}
	svc.wg.Wait()

	if searches, gets := provider.fakeProvider.counts(); searches != 1 || gets != 1 {
		t.Fatalf("fallback lookups = %d searches, %d gets, want one of each", searches, gets)
	}
	for _, game := range []catalog.Game{resolved, missed} {
		stored, err := cat.GetGame(game.ID)
		if err != nil {
			t.Fatalf("reload %s: %v", game.Title, err)
		}
		if stored.CoverAssetID == "" {
			t.Fatalf("%s has no cover", game.Title)
		}
	}
}

func TestEnsureArtRejectsAForeignGameFromTheBatch(t *testing.T) {
	srv := imageServer(t)
	provider := newBatchProvider(t, srv.URL, "Adorable Adventures")
	foreign := provider.resolve["Adorable Adventures"]
	foreign.Title = "The Incredible Adventures of Van Helsing"
	foreign.ProviderID = "2638"
	provider.resolve["Adorable Adventures"] = foreign
	svc, cat, _ := newTestService(t, provider)

	game := addGame(t, cat, catalog.Game{Title: "Adorable Adventures", Provisional: true})

	if _, err := svc.EnsureArt([]string{game.ID}); err != nil {
		t.Fatalf("ensure art: %v", err)
	}
	svc.wg.Wait()

	stored, err := cat.GetGame(game.ID)
	if err != nil {
		t.Fatalf("reload game: %v", err)
	}
	if stored.Title != "Adorable Adventures" {
		t.Fatalf("title = %q, want the game left under its own name", stored.Title)
	}
	if stored.ExternalIDs.IGDB != "" || stored.CoverAssetID != "" {
		t.Fatalf("foreign metadata applied: %+v", stored)
	}
	if len(stored.Aliases) != 0 {
		t.Fatalf("aliases = %v, want none", stored.Aliases)
	}
}

func TestEnsureArtFallsBackWhenTheBatchFails(t *testing.T) {
	srv := imageServer(t)
	provider := newBatchProvider(t, srv.URL, "Prey")
	provider.resolveErr = errors.New("batch endpoint is down")
	provider.candidates = []Candidate{{ProviderID: "2657", Title: "Prey"}}
	svc, cat, _ := newTestService(t, provider)

	game := addGame(t, cat, catalog.Game{Title: "Prey"})

	if _, err := svc.EnsureArt([]string{game.ID}); err != nil {
		t.Fatalf("ensure art: %v", err)
	}
	svc.wg.Wait()

	stored, err := cat.GetGame(game.ID)
	if err != nil {
		t.Fatalf("reload game: %v", err)
	}
	if stored.CoverAssetID == "" {
		t.Fatal("cover not stored through the per-game fallback")
	}
}

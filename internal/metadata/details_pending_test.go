package metadata

import "testing"

// TestPendingDetailsAreAskedForAgainOnTheNextOpen fails against a launcher that
// stores a "details pending" answer as complete: it then keeps a game without
// its developer and publisher for the whole week-long metadata lifetime.
func TestPendingDetailsAreAskedForAgainOnTheNextOpen(t *testing.T) {
	srv := imageServer(t)
	pending := fullMetadata(srv.URL)
	pending.Developer, pending.Publisher = "", ""
	pending.DetailsPending = true
	provider := &fakeProvider{meta: map[string]GameMetadata{"2657": pending}}
	svc, cat, _ := newTestService(t, provider)
	game := resolvedGame(t, svc, cat, pending)

	stored, err := cat.GetGame(game.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !stored.MetadataPartial {
		t.Fatal("a pending answer was stored as complete")
	}

	complete := fullMetadata(srv.URL)
	provider.mu.Lock()
	provider.meta["2657"] = complete
	provider.mu.Unlock()
	_, getsBefore := provider.counts()
	started, err := svc.EnsureFresh(game.ID)
	if err != nil || !started {
		t.Fatalf("the next open did not ask again: started=%v err=%v", started, err)
	}
	svc.wg.Wait()
	if _, gets := provider.counts(); gets != getsBefore+1 {
		t.Fatalf("gets = %d, want %d", gets, getsBefore+1)
	}
	stored, err = cat.GetGame(game.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.MetadataPartial || stored.Developer != complete.Developer {
		t.Fatalf("after the complete answer: partial=%v developer=%q", stored.MetadataPartial, stored.Developer)
	}

	started, err = svc.EnsureFresh(game.ID)
	if err != nil || started {
		t.Fatalf("a complete game was asked for again: started=%v err=%v", started, err)
	}
}

package cloudsaves

import (
	"testing"

	"typhon/internal/account"
)

func gamesFor(st Status, gameID string) []GameStatus {
	var out []GameStatus
	for _, g := range st.Games {
		if g.GameID == gameID {
			out = append(out, g)
		}
	}
	return out
}

func wantListedConflict(t *testing.T, st Status, want *Conflict) {
	t.Helper()
	got := gamesFor(st, gameLocal)
	if len(got) != 1 || got[0].State != StateConflict || got[0].Conflict == nil {
		t.Fatalf("status games for %s = %+v, want one conflict with its details", gameLocal, got)
	}
	if *got[0].Conflict != *want {
		t.Fatalf("listed conflict = %+v, want the recorded %+v", *got[0].Conflict, *want)
	}
}

// A conflict recorded in an earlier session is what the launch hook blocks
// on from the first Play: the status shows it from the start, with the
// details the dialog needs, without a pass and without the network.
func TestStatusListsAConflictRecordedEarlierBeforeAnyPass(t *testing.T) {
	for _, tc := range []struct {
		name string
		arm  func(h *harness)
	}{
		{"cloud unreachable", func(h *harness) {
			h.client.listErr = &account.Error{Code: account.CodeNetwork}
			h.pass()
		}},
		{"cloud never asked", func(*harness) {}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t)
			h.recordConflict()
			want := h.persisted(gameCloud).Conflict
			h.restart()
			calls := h.client.calls()
			tc.arm(h)

			st := h.svc.Status()

			wantListedConflict(t, st, want)
			if tc.name == "cloud never asked" && h.client.calls() != calls {
				t.Fatalf("%d cloud requests for the status, want none", h.client.calls()-calls)
			}
			if tc.name == "cloud unreachable" && st.Error != codeNetwork {
				t.Fatalf("status error = %q, want %s from the failed pass", st.Error, codeNetwork)
			}
			for _, ev := range h.drain() {
				if ev.GameID == gameLocal {
					t.Fatalf("event %+v: reading the status announces nothing", ev)
				}
			}
		})
	}
}

// The status follows the stored credential's account, as the launch gate
// does: another account's recorded conflict is not shown.
func TestStatusDoesNotListAnotherAccountsConflict(t *testing.T) {
	t.Run("conflict recorded for the other account", func(t *testing.T) {
		h := newHarness(t)
		writeTree(t, h.saves(), baseFiles)
		h.pass()
		st := syncState{DeviceID: "device", Games: map[string]gameState{
			gameCloud: {Conflict: &Conflict{LocalDigest: "local", RemoteDigest: "remote"}},
		}}
		if err := h.svc.store.save(otherAccount, st); err != nil {
			t.Fatalf("write the other account's state: %v", err)
		}
		h.restart()

		if got := gamesFor(h.svc.Status(), gameLocal); len(got) != 0 {
			t.Fatalf("status games = %+v, want none: the conflict belongs to another account", got)
		}
	})
	t.Run("stored credential names another account", func(t *testing.T) {
		h := newHarness(t)
		h.recordConflict()
		h.restart()
		h.setStored(otherAccount)

		if got := gamesFor(h.svc.Status(), gameLocal); len(got) != 0 {
			t.Fatalf("status games = %+v, want none: the conflict belongs to %s", got, ownerID)
		}
	})
	for _, tc := range []struct {
		name string
		arm  func(h *harness)
	}{
		{"signed out", func(h *harness) { h.setToken("") }},
		{"credential rejected", func(h *harness) { h.setStored("") }},
		{"cloud saves off", func(h *harness) { h.setEnabled(false) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t)
			h.recordConflict()
			h.restart()
			tc.arm(h)

			if got := gamesFor(h.svc.Status(), gameLocal); len(got) != 0 {
				t.Fatalf("status games = %+v, want none: the launch gate does not hold the game either", got)
			}
		})
	}
}

// Once a pass reaches the cloud the entry is the one the pass records: one
// entry, its details refreshed from the saves as they are now.
func TestPassAfterAListedConflictKeepsOneEntry(t *testing.T) {
	h := newHarness(t)
	h.recordConflict()
	h.restart()
	wantListedConflict(t, h.svc.Status(), h.persisted(gameCloud).Conflict)

	writeTree(t, h.saves(), map[string]string{"slot1.sav": "one, played even further", "profile/slot2.sav": "two"})
	h.pass()

	refreshed := h.persisted(gameCloud).Conflict
	if refreshed == nil || refreshed.LocalDigest == "" {
		t.Fatalf("persisted conflict = %+v, want it kept", refreshed)
	}
	wantListedConflict(t, h.svc.Status(), refreshed)
}

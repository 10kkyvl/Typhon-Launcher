package cloudsaves

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"typhon/internal/account"
	"typhon/internal/library"
	"typhon/internal/uierr"
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
	// The state of the account the session ran with stays in memory when the
	// stored credential changes without a restart: the status must not read
	// it for the new account. The gate does not hold the game either.
	for _, inMemory := range []bool{false, true} {
		t.Run(fmt.Sprintf("stored credential names another account, session unconfirmed, state in memory %v", inMemory), func(t *testing.T) {
			h := newHarness(t)
			h.recordConflict()
			h.restart()
			if inMemory {
				// A pass that loads the state and fails at the listing leaves
				// no status of the game: only the state holds the conflict.
				h.client.listErr = &account.Error{Code: account.CodeNetwork}
				h.pass()
			}
			h.setStored(otherAccount)
			h.setAccountErr(errors.New("account id is not bound to the session yet"))

			if got := gamesFor(h.svc.Status(), gameLocal); len(got) != 0 {
				t.Fatalf("status games = %+v, want none: the conflict belongs to %s", got, ownerID)
			}
			wantStarts(t, h.starting(gameLocal))
		})
	}
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

// The invariant behind the dialog: whenever the launch gate holds a game on
// its recorded conflict, the status lists that game as a conflict with the
// recorded details, whatever this session has seen of the game meanwhile.
func TestStatusListsEveryConflictTheLaunchGateBlocksOn(t *testing.T) {
	for _, tc := range []struct {
		name string
		arm  func(h *harness)
	}{
		{"restarted, no pass", func(h *harness) { h.restart() }},
		{"restarted, the pass could not reach the cloud", func(h *harness) {
			h.restart()
			h.client.listErr = &account.Error{Code: account.CodeNetwork}
			h.pass()
		}},
		{"the pass failed for the game: saves folder unreadable", func(h *harness) {
			h.lib.mu.Lock()
			h.lib.results[gameLocal] = library.SavesResult{Unreadable: 1}
			h.lib.mu.Unlock()
			h.pass()
		}},
		{"the pass failed for the game: network lost during its download", func(h *harness) {
			writeTree(h.t, h.saves(), baseFiles)
			h.client.downloadErr = &account.Error{Code: account.CodeNetwork}
			h.pass()
		}},
		{"the pass skipped the game: saves folder not found", func(h *harness) {
			h.lib.mu.Lock()
			h.lib.results[gameLocal] = library.SavesResult{}
			h.lib.mu.Unlock()
			h.pass()
		}},
		{"the pass skipped the game: it was running", func(h *harness) {
			h.lib.setRunning(gameLocal, true)
			h.pass()
			h.lib.setRunning(gameLocal, false)
		}},
		{"stored credential names another account, the confirmed one holds the conflict", func(h *harness) {
			h.restart()
			h.setStored(otherAccount)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t)
			h.recordConflict()
			h.open()
			limitLaunch(h, time.Minute)
			tc.arm(h)
			recorded := h.persisted(gameCloud).Conflict
			if recorded == nil {
				t.Fatal("setup: the conflict is no longer recorded")
			}

			st := h.svc.Status()
			wantBlock(t, h.starting(gameLocal), wantConflictCode)

			wantListedConflict(t, st, recorded)
		})
	}
}

// A pass records what it found for a game; the listing only adds the
// recorded conflict over what does not show it.
func TestPassOutcomesOfAConflictedGameStayAsRecorded(t *testing.T) {
	h := newHarness(t)
	h.recordConflict()
	h.open()
	h.lib.mu.Lock()
	h.lib.results[gameLocal] = library.SavesResult{Unreadable: 1}
	h.lib.mu.Unlock()

	h.pass()

	evs := eventsFor(h.drain(), gameLocal)
	if len(evs) != 1 || evs[0].Status != StateFailed || evs[0].Code != codeSavesUnread {
		t.Fatalf("events = %+v, want the pass's one failed %s", evs, codeSavesUnread)
	}
	if err := h.svc.SyncNow(gameLocal); uierr.Code(err) != codeSavesUnread {
		t.Fatalf("SyncNow = %v, want %s as the pass records it", err, codeSavesUnread)
	}
}

// The gate lets every entry of a duplicated catalog id start, so the
// listing does not show their recorded conflict; a pass reports them as
// duplicates, as before.
func TestStatusDoesNotListAConflictOfADuplicatedGame(t *testing.T) {
	h := newHarness(t)
	h.recordConflict()
	writeTree(t, h.addGame("local-2", gameCloud), baseFiles)
	h.restart()

	for _, id := range []string{gameLocal, "local-2"} {
		if got := gamesFor(h.svc.Status(), id); len(got) != 0 {
			t.Fatalf("status games for %s = %+v, want none: the gate lets a duplicated game start", id, got)
		}
		wantStarts(t, h.starting(id))
	}

	h.pass()
	for _, id := range []string{gameLocal, "local-2"} {
		if g := h.status(id); g.State != StateSkipped || g.Code != codeDuplicate || g.Conflict != nil {
			t.Fatalf("status of %s after a pass = %+v, want skipped %s", id, g, codeDuplicate)
		}
	}
}

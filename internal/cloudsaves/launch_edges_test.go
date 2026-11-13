package cloudsaves

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"
)

// Case 6: the player cancels while nothing is known yet. The check goes with
// the launch, so the late listing finds nobody to download for.
func TestCancelledLaunchBeforeAnythingIsKnownLeavesNothingRunning(t *testing.T) {
	h := newHarness(t)
	h.newerInCloud()
	h.open()
	limitLaunch(h, time.Minute)
	releaseList := h.client.hold(listKey)
	t.Cleanup(releaseList)
	before := h.stateBytes()
	recorded := h.recorded(gameCloud)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := h.startingAsync(ctx, gameLocal)
	h.client.waitHeld(t, listKey)
	cancel()
	if err := await(t, done, "the cancelled SessionStarting"); err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("SessionStarting = %v, want nil or context.Canceled", err)
	}
	releaseList()
	waitFor(t, "the launch check to end", func() bool { return !h.svc.launchCheckRuns(gameLocal) })

	if _, d := h.client.transfers(); d != 0 || h.backups.restoreCount() != 0 {
		t.Fatalf("%d downloads and %d restores after a cancelled launch", d, h.backups.restoreCount())
	}
	if got := readTree(t, h.saves()); !mapsEqual(got, baseFiles) {
		t.Fatalf("saves changed to %v", got)
	}
	if got := h.recorded(gameCloud); got != recorded {
		t.Fatalf("recorded %+v, want %+v untouched", got, recorded)
	}
	if got := h.stateBytes(); !bytes.Equal(got, before) {
		t.Fatalf("state file changed:\n%s", got)
	}
}

const otherAccount = "account-2"

// storedOther makes the stored credential name otherAccount while the backend
// confirms ownerID, with conflicts recorded per account as asked.
func (h *harness) storedOther(conflictForStored, conflictForConfirmed bool) {
	h.t.Helper()
	if conflictForConfirmed {
		h.recordConflict()
	} else {
		writeTree(h.t, h.saves(), baseFiles)
		h.pass()
		h.drain()
	}
	if conflictForStored {
		st := syncState{DeviceID: "device", Games: map[string]gameState{
			gameCloud: {Conflict: &Conflict{LocalDigest: "local", RemoteDigest: "remote"}},
		}}
		if err := h.svc.store.save(otherAccount, st); err != nil {
			h.t.Fatalf("write the other account's state: %v", err)
		}
	}
	h.setStored(otherAccount)
	h.open()
	limitLaunch(h, time.Minute)
}

// The stored credential and the confirmed session name different accounts:
// a conflict recorded for either one holds the launch, without the network.
func TestConflictOfEitherAccountBlocksWhenTheyDiffer(t *testing.T) {
	for _, tc := range []struct {
		name              string
		stored, confirmed bool
	}{
		{"recorded for the stored account", true, false},
		{"recorded for the confirmed account", false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t)
			h.storedOther(tc.stored, tc.confirmed)
			calls := h.client.calls()

			wantBlock(t, h.starting(gameLocal), wantConflictCode)

			if n := h.client.calls() - calls; n != 0 {
				t.Fatalf("%d cloud requests, want none: the conflict is known from the state file", n)
			}
		})
	}
}

// Two launches that both pass the gates before either has started a check
// share the one check the first of them starts.
func TestLaunchesRacingPastTheGatesShareOneCheck(t *testing.T) {
	h := newHarness(t)
	remote := h.newerInCloud()
	h.open()
	limitLaunch(h, time.Minute)
	lists := h.client.requestsFor(listKey)
	game := h.game(gameLocal)
	svc := h.svc

	entered, release := h.lib.holdGames()
	t.Cleanup(release)
	results := make(chan error, 2)
	for range 2 {
		h.lib.launching(gameLocal, 1)
		go func() {
			err := svc.SessionStarting(context.Background(), game)
			h.lib.launching(gameLocal, -1)
			results <- err
		}()
	}
	for range 2 {
		select {
		case <-entered:
		case <-time.After(watchdog):
			t.Fatal("the launches did not reach the gates")
		}
	}
	// The listing is held so the first check still runs when the second
	// launch leaves the gates.
	releaseList := h.client.hold(listKey)
	t.Cleanup(releaseList)
	release()
	waitFor(t, "both launches to wait on one check", func() bool {
		n, _ := h.svc.launchWaiters(gameLocal)
		return n == 2
	})
	releaseList()

	for i := range 2 {
		if err := await(t, results, "SessionStarting"); err != nil {
			t.Fatalf("launch %d = %v, want nil", i+1, err)
		}
	}
	if n := h.client.requestsFor(listKey) - lists; n != 1 {
		t.Fatalf("%d listings for two launches, want one shared check", n)
	}
	if st := h.recorded(gameCloud); st.RemoteDigest != remote {
		t.Fatalf("recorded %+v", st)
	}
}

func wantNotStarted(t *testing.T, h *harness, err error, calls int) {
	t.Helper()
	wantStarts(t, err)
	if !errors.Is(err, errNotStarted) {
		t.Fatalf("SessionStarting = %v, want a plain error carrying %v", err, errNotStarted)
	}
	if n := h.client.calls() - calls; n != 0 {
		t.Fatalf("%d cloud requests, want none", n)
	}
	if h.svc.launchCheckRuns(gameLocal) {
		t.Fatal("a launch check runs on a service that is not running")
	}
	stopped := make(chan error, 1)
	go func() { stopped <- h.svc.ServiceShutdown() }()
	if err := await(t, stopped, "ServiceShutdown"); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
}

func TestLaunchOnAServiceThatNeverStartedStartsTheGame(t *testing.T) {
	h := newHarness(t)
	h.newerInCloud()
	limitLaunch(h, time.Minute)
	calls := h.client.calls()

	wantNotStarted(t, h, h.starting(gameLocal), calls)
}

func TestLaunchOnAServiceThatShutDownStartsTheGame(t *testing.T) {
	h := newHarness(t)
	h.newerInCloud()
	h.open()
	limitLaunch(h, time.Minute)
	if err := h.svc.ServiceShutdown(); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
	calls := h.client.calls()

	wantNotStarted(t, h, h.starting(gameLocal), calls)
}

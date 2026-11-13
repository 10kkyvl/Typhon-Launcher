package cloudsaves

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"typhon/internal/account"
	"typhon/internal/library"
	"typhon/internal/uierr"
)

// offlineWithConflict is a launcher started with a conflict recorded in an
// earlier session and a cloud that cannot be reached.
func (h *harness) offlineWithConflict() (remote string) {
	h.t.Helper()
	remote = h.recordConflict()
	h.client.listErr = &account.Error{Code: account.CodeNetwork}
	h.restart()
	limitLaunch(h, time.Minute)
	return remote
}

func (h *harness) allow(gameID string) {
	h.t.Helper()
	if err := h.svc.AllowLaunchOnce(gameID); err != nil {
		h.t.Fatalf("AllowLaunchOnce(%s) = %v, want the permission granted", gameID, err)
	}
}

// The player chose to play on this PC's saves while the cloud could not be
// reached: exactly the next launch starts, without asking the cloud, and the
// conflict stays recorded and blocks the one after it.
func TestAllowLaunchOnceLetsOneLaunchStartOnTheLocalSaves(t *testing.T) {
	for _, tc := range []struct {
		name string
		arm  func(h *harness)
	}{
		{"session confirmed", func(*harness) {}},
		{"session not confirmed yet", func(h *harness) {
			h.setAccountErr(errors.New("account id is not bound to the session yet"))
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t)
			remote := h.offlineWithConflict()
			tc.arm(h)
			before := h.stateBytes()
			h.allow(gameLocal)
			calls := h.client.calls()

			if err := h.starting(gameLocal); err != nil {
				t.Fatalf("the allowed launch = %v, want nil", err)
			}

			if n := h.client.calls() - calls; n != 0 {
				t.Fatalf("%d cloud requests for the allowed launch, want none", n)
			}
			if got := readTree(t, h.saves()); !mapsEqual(got, changedBase) {
				t.Fatalf("saves = %v, want the ones on this PC", got)
			}
			if got := h.stateBytes(); !bytes.Equal(got, before) {
				t.Fatalf("state file changed:\n%s", got)
			}
			if c := h.persisted(gameCloud).Conflict; c == nil || c.RemoteDigest != remote {
				t.Fatalf("persisted conflict = %+v, want it kept", c)
			}
			for _, ev := range eventsFor(h.drain(), gameLocal) {
				t.Fatalf("event %+v: the permission announces nothing", ev)
			}

			wantBlock(t, h.starting(gameLocal), wantConflictCode)
		})
	}
}

func TestAllowLaunchOnceIsRefusedWhereNoConflictHoldsTheLaunch(t *testing.T) {
	for _, tc := range []struct {
		name string
		game string
		arm  func(h *harness)
		want string
	}{
		{"no conflict recorded", gameLocal, func(h *harness) {
			writeTree(h.t, h.saves(), baseFiles)
			h.pass()
		}, codeNoConflict},
		{"conflict of another game only", "local-2", func(h *harness) {
			h.addGame("local-2", "cloud-2")
			h.recordConflict()
		}, codeNoConflict},
		{"unknown game", "no-such-game", func(h *harness) { h.recordConflict() }, codeGameNotFound},
		{"cloud saves off", gameLocal, func(h *harness) {
			h.recordConflict()
			h.setEnabled(false)
		}, codeDisabled},
		{"signed out", gameLocal, func(h *harness) {
			h.recordConflict()
			h.setToken("")
		}, codeNotSignedIn},
		{"credential rejected", gameLocal, func(h *harness) {
			h.recordConflict()
			h.setStored("")
		}, codeNotSignedIn},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t)
			tc.arm(h)
			h.drain()

			err := h.svc.AllowLaunchOnce(tc.game)

			if code := uierr.Code(err); code != tc.want {
				t.Fatalf("AllowLaunchOnce = %v (code %q), want %s", err, code, tc.want)
			}
			if h.svc.permitHeld(tc.game) {
				t.Fatal("a refused permission is held")
			}
			if evs := h.drain(); len(evs) != 0 {
				t.Fatalf("events = %+v, want none", evs)
			}
		})
	}
}

// A download of the game's cloud copy over its saves holds every launch: the
// permission is for a conflict left as it is, never for saves being replaced.
func TestAllowedLaunchIsStillHeldWhileTheSavesAreReplaced(t *testing.T) {
	h := newHarness(t)
	h.recordConflict()
	h.open()
	limitLaunch(h, shortLimit)
	h.allow(gameLocal)
	release := h.client.hold(gameCloud)
	var resolveErr error
	resolved := make(chan struct{})
	go func() {
		defer close(resolved)
		resolveErr = h.svc.ResolveConflict(gameLocal, false)
	}()
	t.Cleanup(func() {
		release()
		<-resolved
	})
	h.client.waitHeld(t, gameCloud)

	wantBlock(t, h.starting(gameLocal), wantConflictCode, wantSyncingCode)

	release()
	awaitClosed(t, resolved, "ResolveConflict")
	if resolveErr != nil {
		t.Fatalf("ResolveConflict: %v", resolveErr)
	}
	if got := readTree(t, h.saves()); !mapsEqual(got, otherDevice) {
		t.Fatalf("saves = %v, want the kept cloud copy", got)
	}
}

// The permission does not outlive the launch it was given for: a repeat that
// never reached the hook leaves nothing a later Play could use.
func TestLaunchPermissionExpires(t *testing.T) {
	h := newHarness(t)
	h.offlineWithConflict()
	h.allow(gameLocal)

	h.advance(launchPermitTTL)

	wantBlock(t, h.starting(gameLocal), wantConflictCode)
	if h.svc.permitHeld(gameLocal) {
		t.Fatal("an expired permission is still held")
	}
}

func TestLaunchPermissionIsDroppedWithItsConflict(t *testing.T) {
	t.Run("resolved", func(t *testing.T) {
		h := newHarness(t)
		h.offlineWithConflict()
		h.allow(gameLocal)
		h.client.listErr = nil

		if err := h.svc.ResolveConflict(gameLocal, true); err != nil {
			t.Fatalf("ResolveConflict: %v", err)
		}

		if h.svc.permitHeld(gameLocal) {
			t.Fatal("the permission outlived the conflict it was given for")
		}
	})
	t.Run("replaced by a newer one", func(t *testing.T) {
		h := newHarness(t)
		h.offlineWithConflict()
		h.allow(gameLocal)
		h.client.listErr = nil
		writeTree(t, h.saves(), map[string]string{"slot1.sav": "one, played even further", "profile/slot2.sav": "two"})

		h.pass()

		if h.recorded(gameCloud).Conflict == nil {
			t.Fatal("setup: the pass recorded no conflict")
		}
		if h.svc.permitHeld(gameLocal) {
			t.Fatal("the permission outlived the conflict it was given for")
		}
		wantBlock(t, h.starting(gameLocal), wantConflictCode)
	})
}

// Two launches racing with one permission: the check and the take are one
// step, so one starts and the other is held.
func TestOnePermissionOpensOneOfTwoConcurrentLaunches(t *testing.T) {
	h := newHarness(t)
	h.offlineWithConflict()
	h.allow(gameLocal)
	game := h.game(gameLocal)
	svc := h.svc

	entered, release := h.lib.holdGames()
	t.Cleanup(release)
	results := make(chan error, 2)
	for range 2 {
		go func() { results <- svc.SessionStarting(context.Background(), game) }()
	}
	for range 2 {
		select {
		case <-entered:
		case <-time.After(watchdog):
			t.Fatal("the launches did not reach the gates")
		}
	}
	release()

	var started, held int
	for range 2 {
		err := await(t, results, "SessionStarting")
		var block *library.LaunchBlockedError
		switch {
		case err == nil:
			started++
		case errors.As(err, &block) && uierr.Code(block.Err) == wantConflictCode:
			held++
		default:
			t.Fatalf("SessionStarting = %v, want nil or the conflict block", err)
		}
	}
	if started != 1 || held != 1 {
		t.Fatalf("%d launches started and %d held, want one of each", started, held)
	}
}

// A session played with the permission changes nothing in the cloud logic:
// once the cloud is back the pass records the conflict with the saves as they
// are now, and uploads and downloads nothing.
func TestSessionPlayedWithThePermissionLeavesTheConflictToThePlayer(t *testing.T) {
	h := newHarness(t)
	remote := h.offlineWithConflict()
	h.allow(gameLocal)
	if err := h.starting(gameLocal); err != nil {
		t.Fatalf("the allowed launch = %v", err)
	}
	played := map[string]string{"slot1.sav": "one, played offline", "profile/slot2.sav": "two"}
	writeTree(t, h.saves(), played)
	uploads, downloads := h.client.transfers()
	h.client.listErr = nil

	h.pass()

	if u, d := h.client.transfers(); u != uploads || d != downloads {
		t.Fatalf("uploads +%d downloads +%d after the session, want none", u-uploads, d-downloads)
	}
	c := h.persisted(gameCloud).Conflict
	if c == nil || c.RemoteDigest != remote || c.LocalDigest != contractDigest(played) {
		t.Fatalf("persisted conflict = %+v, want it kept with the saves played offline", c)
	}
	if rs, ok := h.client.remoteOf(gameCloud); !ok || rs.save.Digest != remote {
		t.Fatalf("cloud copy changed to %+v", rs.save)
	}
	if got := readTree(t, h.saves()); !mapsEqual(got, played) {
		t.Fatalf("saves = %v, want the ones played offline", got)
	}
	wantBlock(t, h.starting(gameLocal), wantConflictCode)
}

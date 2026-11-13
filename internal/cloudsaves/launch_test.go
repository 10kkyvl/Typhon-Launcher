package cloudsaves

// Assumes the implementer's names; each one appears on exactly one line below,
// adapt that line:
//   - codeConflictUnresolved: new code in model.go, "the conflict is unresolved" (wantConflictCode)
//   - codeSyncing: new code in model.go, "the saves are still syncing" (wantSyncingCode)
//   - Service.launchTimeout time.Duration: the hook's time limit (limitLaunch)
//   - (*Service).SessionStarting(context.Context, library.Game) error (the LaunchHook assertion)

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"typhon/internal/account"
	"typhon/internal/library"
	"typhon/internal/savebackup"
	"typhon/internal/uierr"
)

var _ library.LaunchHook = (*Service)(nil)

const (
	wantConflictCode = codeConflictUnresolved
	wantSyncingCode  = codeSyncing
)

func limitLaunch(h *harness, d time.Duration) { h.svc.launchTimeout = d }

// shortLimit stands for the hook's time limit wherever a held transfer must
// outlast it: long enough for an unheld listing and hash under -race, far
// below the watchdog.
const shortLimit = 500 * time.Millisecond

// watchdog bounds every wait in these tests: a hook that waits behind a lock
// or a transfer fails the test with a message instead of hanging it.
const watchdog = 10 * time.Second

func await(t *testing.T, ch <-chan error, what string) error {
	t.Helper()
	timer := time.NewTimer(watchdog)
	defer timer.Stop()
	select {
	case err := <-ch:
		return err
	case <-timer.C:
		t.Fatalf("%s did not return within %v", what, watchdog)
		return nil
	}
}

func awaitClosed(t *testing.T, ch <-chan struct{}, what string) {
	t.Helper()
	timer := time.NewTimer(watchdog)
	defer timer.Stop()
	select {
	case <-ch:
	case <-timer.C:
		t.Fatalf("%s did not finish within %v", what, watchdog)
	}
}

func (h *harness) game(id string) library.Game {
	h.t.Helper()
	for _, g := range h.lib.GetGames() {
		if g.ID == id {
			return g
		}
	}
	h.t.Fatalf("no game %s in the library", id)
	return library.Game{}
}

func (h *harness) startingAsync(ctx context.Context, id string) <-chan error {
	h.t.Helper()
	svc, g := h.svc, h.game(id)
	done := make(chan error, 1)
	go func() { done <- svc.SessionStarting(ctx, g) }()
	return done
}

func (h *harness) starting(id string) error {
	h.t.Helper()
	return await(h.t, h.startingAsync(context.Background(), id), "SessionStarting("+id+")")
}

func blockedCode(t *testing.T, err error) string {
	t.Helper()
	var blocked *library.LaunchBlockedError
	if !errors.As(err, &blocked) || blocked.Err == nil {
		t.Fatalf("SessionStarting = %v (%T), want a *library.LaunchBlockedError: any other result lets the library start the game", err, err)
	}
	return uierr.Code(blocked.Err)
}

func wantBlock(t *testing.T, err error, codes ...string) {
	t.Helper()
	if got := blockedCode(t, err); !slices.Contains(codes, got) {
		t.Fatalf("launch blocked with code %q, want one of %v (err %v)", got, codes, err)
	}
}

// wantStarts accepts nil or a plain error: the library logs a plain hook error
// and starts the game anyway.
func wantStarts(t *testing.T, err error) {
	t.Helper()
	var blocked *library.LaunchBlockedError
	if errors.As(err, &blocked) {
		t.Fatalf("SessionStarting = %v, want the game to start", err)
	}
}

func (h *harness) stateBytes() []byte {
	h.t.Helper()
	data, err := os.ReadFile(h.svc.store.path(ownerID))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		h.t.Fatalf("read state file: %v", err)
	}
	return data
}

func (h *harness) persisted(key string) gameState {
	h.t.Helper()
	st, err := h.svc.store.load(ownerID)
	if err != nil {
		h.t.Fatalf("load state file: %v", err)
	}
	return st.Games[key]
}

// recordConflict leaves gameLocal with a conflict recorded by a pass: changed
// here, changed in the cloud by another device.
func (h *harness) recordConflict() string {
	h.t.Helper()
	writeTree(h.t, h.saves(), baseFiles)
	h.pass()
	writeTree(h.t, h.saves(), changedBase)
	remote := h.seedRemote(otherDevice, "other-device")
	h.pass()
	if h.recorded(gameCloud).Conflict == nil {
		h.t.Fatal("setup: the pass recorded no conflict")
	}
	h.drain()
	return remote
}

// newerInCloud leaves gameLocal synced once, unchanged here since, and a newer
// copy from another device in the cloud.
func (h *harness) newerInCloud() string {
	h.t.Helper()
	writeTree(h.t, h.saves(), baseFiles)
	h.pass()
	if st := h.recorded(gameCloud); st.RemoteDigest != contractDigest(baseFiles) {
		h.t.Fatalf("setup: first sync recorded %+v", st)
	}
	remote := h.seedRemote(otherDevice, "other-device")
	h.drain()
	return remote
}

func eventsFor(evs []Event, gameID string) []Event {
	var out []Event
	for _, ev := range evs {
		if ev.GameID == gameID {
			out = append(out, ev)
		}
	}
	return out
}

type logSink struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (l *logSink) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.Write(p)
}

func (l *logSink) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.String()
}

func captureLogs(t *testing.T) *logSink {
	t.Helper()
	sink := &logSink{}
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(sink, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return sink
}

func TestLaunchBlockCodesAreNewAndDistinct(t *testing.T) {
	for _, code := range []string{wantConflictCode, wantSyncingCode} {
		if !strings.HasPrefix(code, "cloud.") || code == codeConflictHTTP || code == codeBusy {
			t.Fatalf("launch code %q must be a new cloud.* code, not the HTTP 409 or busy one", code)
		}
	}
	if wantConflictCode == wantSyncingCode {
		t.Fatal("the conflict and the syncing launch blocks share one code")
	}
}

// Case 1.
func TestLaunchWithARecordedConflictIsBlockedWithoutTheNetwork(t *testing.T) {
	h := newHarness(t)
	remoteDigest := h.recordConflict()
	h.client.listErr = &account.Error{Code: account.CodeNetwork}
	h.restart()
	limitLaunch(h, time.Minute)
	before := h.stateBytes()
	uploads, downloads := h.client.transfers()

	err := h.starting(gameLocal)

	wantBlock(t, err, wantConflictCode)
	if u, d := h.client.transfers(); u != uploads || d != downloads || h.backups.restoreCount() != 0 {
		t.Fatalf("a blocked launch moved data: uploads +%d downloads +%d restores %d", u-uploads, d-downloads, h.backups.restoreCount())
	}
	if got := readTree(t, h.saves()); !mapsEqual(got, changedBase) {
		t.Fatalf("local saves changed to %v", got)
	}
	if rs, ok := h.client.remoteOf(gameCloud); !ok || rs.save.Digest != remoteDigest {
		t.Fatalf("cloud copy changed to %+v", rs.save)
	}
	if got := h.stateBytes(); !bytes.Equal(got, before) {
		t.Fatalf("state file rewritten by a launch check:\n%s\nwas\n%s", got, before)
	}
	if c := h.persisted(gameCloud).Conflict; c == nil || c.RemoteDigest != remoteDigest {
		t.Fatalf("persisted conflict = %+v, want it kept", c)
	}
	for _, ev := range eventsFor(h.drain(), gameLocal) {
		if ev.Status != StateConflict {
			t.Fatalf("event %+v, a game with a recorded conflict is announced only as a conflict", ev)
		}
	}
}

// Case 2, saves unchanged here.
func TestLaunchDownloadsANewerCloudCopyBeforeTheGameStarts(t *testing.T) {
	h := newHarness(t)
	remoteDigest := h.newerInCloud()
	h.open()
	limitLaunch(h, time.Minute)

	err := h.starting(gameLocal)

	if err != nil {
		t.Fatalf("SessionStarting = %v, want nil once the newer saves are in place", err)
	}
	if got := readTree(t, h.saves()); !mapsEqual(got, otherDevice) {
		t.Fatalf("saves when the game starts = %v, want the cloud copy %v", got, otherDevice)
	}
	if uploads, downloads := h.client.transfers(); downloads != 1 || uploads != 1 {
		t.Fatalf("downloads %d uploads %d, want one download and only the first sync's upload", downloads, uploads)
	}
	if len(h.backups.snapshots) != 1 || h.backups.snapshots[0].kind != savebackup.KindCloud || h.backups.snapshots[0].gameID != gameLocal {
		t.Fatalf("snapshots = %+v, want the cloud copy staged through savebackup", h.backups.snapshots)
	}
	if n := h.backups.restoreCount(); n != 1 {
		t.Fatalf("%d restores, want 1", n)
	}
	for name, st := range map[string]gameState{"memory": h.recorded(gameCloud), "state file": h.persisted(gameCloud)} {
		if st.LocalDigest != remoteDigest || st.RemoteDigest != remoteDigest || st.Conflict != nil {
			t.Fatalf("%s = %+v, want both digests %s and no conflict", name, st, remoteDigest)
		}
	}
	for _, ev := range eventsFor(h.drain(), gameLocal) {
		if ev.Status == StateConflict || ev.Status == StateFailed {
			t.Fatalf("event %+v after a clean download", ev)
		}
	}
}

// Case 2, saves changed on both sides.
func TestLaunchRecordsANewConflictAsAPassDoesAndIsBlocked(t *testing.T) {
	h := newHarness(t)
	writeTree(t, h.saves(), baseFiles)
	h.pass()
	writeTree(t, h.saves(), changedBase)
	remoteDigest := h.seedRemote(otherDevice, "other-device")
	h.drain()
	h.open()
	limitLaunch(h, time.Minute)
	uploads, _ := h.client.transfers()
	lists := h.client.requestsFor(listKey)

	err := h.starting(gameLocal)

	wantBlock(t, err, wantConflictCode)
	if h.client.requestsFor(listKey) == lists {
		t.Fatal("the cloud was not asked: a conflict nobody recorded can only be found there")
	}
	if u, d := h.client.transfers(); u != uploads || d != 0 || h.backups.restoreCount() != 0 {
		t.Fatalf("a conflict moved data: uploads +%d downloads %d restores %d", u-uploads, d, h.backups.restoreCount())
	}
	if got := readTree(t, h.saves()); !mapsEqual(got, changedBase) {
		t.Fatalf("local saves changed to %v", got)
	}
	if rs, ok := h.client.remoteOf(gameCloud); !ok || rs.save.Digest != remoteDigest {
		t.Fatalf("cloud copy changed to %+v", rs.save)
	}
	local := contractDigest(changedBase)
	for name, st := range map[string]gameState{"memory": h.recorded(gameCloud), "state file": h.persisted(gameCloud)} {
		c := st.Conflict
		if c == nil || c.LocalDigest != local || c.RemoteDigest != remoteDigest || c.RemoteDevice != "other-device" {
			t.Fatalf("%s conflict = %+v, want local %s remote %s from other-device", name, c, local, remoteDigest)
		}
	}
	if evs := eventsFor(h.drain(), gameLocal); len(evs) != 1 || evs[0].Status != StateConflict {
		t.Fatalf("events = %+v, want one conflict event as a pass emits", evs)
	}
	if got := h.status(gameLocal); got.State != StateConflict || got.Conflict == nil {
		t.Fatalf("status = %+v, want the conflict", got)
	}

	h.pass()
	if evs := h.drain(); len(evs) != 0 {
		t.Fatalf("the next pass announced the conflict again (%+v): the launch did not record it the way a pass does", evs)
	}
	wantBlock(t, h.starting(gameLocal), wantConflictCode)
}

// Case 2, I-1..I-3: the conflict cannot be written.
func TestLaunchConflictThatCannotBePersistedBlocksAndLeavesNoMemoryOnlyConflict(t *testing.T) {
	h := newHarness(t)
	writeTree(t, h.saves(), baseFiles)
	h.pass()
	writeTree(t, h.saves(), changedBase)
	h.seedRemote(otherDevice, "other-device")
	h.drain()
	h.open()
	limitLaunch(h, time.Minute)
	before := h.stateBytes()
	realWrite := h.svc.store.write
	h.svc.store.write = func(string, int, any) error { return errors.New("disk full") }

	err := h.starting(gameLocal)

	if code := blockedCode(t, err); !strings.HasPrefix(code, "cloud.") {
		t.Fatalf("blocked with code %q, want a cloud.* code the UI can show", code)
	}
	if c := h.recorded(gameCloud).Conflict; c != nil {
		t.Fatalf("memory holds a conflict the disk never confirmed: %+v", c)
	}
	if got := h.stateBytes(); !bytes.Equal(got, before) {
		t.Fatalf("state file changed although every write failed:\n%s", got)
	}
	if _, d := h.client.transfers(); d != 0 {
		t.Fatalf("%d downloads over saves changed on both sides", d)
	}
	if got := readTree(t, h.saves()); !mapsEqual(got, changedBase) {
		t.Fatalf("local saves changed to %v", got)
	}

	h.svc.store.write = realWrite
	h.pass()
	if c := h.persisted(gameCloud).Conflict; c == nil {
		t.Fatal("the conflict was not recorded once the disk accepted writes again")
	}
}

// Case 3 and the time limit.
func TestLaunchWhileItsDownloadOutlastsTheLimitIsBlockedUntilTheDownloadFinishes(t *testing.T) {
	h := newHarness(t)
	remoteDigest := h.newerInCloud()
	h.open()
	limitLaunch(h, shortLimit)
	transfers := h.client.requestsFor(gameCloud)
	release := h.client.hold(gameCloud)
	t.Cleanup(release)
	before := h.stateBytes()

	wantBlock(t, h.starting(gameLocal), wantSyncingCode)
	h.client.waitHeld(t, gameCloud)

	if got := readTree(t, h.saves()); !mapsEqual(got, baseFiles) {
		t.Fatalf("saves = %v while the download is still held", got)
	}
	wantBlock(t, h.starting(gameLocal), wantSyncingCode)
	if n := h.client.requestsFor(gameCloud) - transfers; n != 1 {
		t.Fatalf("%d transfers of %s, want the one the first launch started", n, gameCloud)
	}
	if got := h.stateBytes(); !bytes.Equal(got, before) {
		t.Fatalf("state file written for a transfer still in flight:\n%s", got)
	}
	if c := h.recorded(gameCloud).Conflict; c != nil {
		t.Fatalf("a running download was recorded as a conflict: %+v", c)
	}

	release()
	waitFor(t, "the background download to be recorded", func() bool {
		st := h.recorded(gameCloud)
		return st.LocalDigest == remoteDigest && st.RemoteDigest == remoteDigest
	})

	if err := h.starting(gameLocal); err != nil {
		t.Fatalf("launch after the download finished = %v, want nil", err)
	}
	if got := readTree(t, h.saves()); !mapsEqual(got, otherDevice) {
		t.Fatalf("saves = %v, want the downloaded cloud copy", got)
	}
	if n := h.client.requestsFor(gameCloud) - transfers; n != 1 {
		t.Fatalf("%d transfers of %s, want 1", n, gameCloud)
	}
	if st := h.persisted(gameCloud); st.LocalDigest != remoteDigest || st.RemoteDigest != remoteDigest {
		t.Fatalf("state file = %+v, want the download recorded", st)
	}
}

// Case 3, the download was started by a background pass.
func TestLaunchWhileAPassDownloadsTheGameIsBlockedAndStartsNoSecondTransfer(t *testing.T) {
	h := newHarness(t)
	remoteDigest := h.newerInCloud()
	h.open()
	limitLaunch(h, shortLimit)
	transfers := h.client.requestsFor(gameCloud)
	release := h.client.hold(gameCloud)
	passDone := make(chan struct{})
	go func() {
		defer close(passDone)
		h.svc.pass(context.Background(), map[string]struct{}{gameLocal: {}})
	}()
	t.Cleanup(func() {
		release()
		<-passDone
	})
	h.client.waitHeld(t, gameCloud)

	wantBlock(t, h.starting(gameLocal), wantSyncingCode)

	release()
	awaitClosed(t, passDone, "the pass")
	if st := h.recorded(gameCloud); st.RemoteDigest != remoteDigest {
		t.Fatalf("recorded %+v, want the pass's download", st)
	}
	if err := h.starting(gameLocal); err != nil {
		t.Fatalf("launch after the pass finished = %v, want nil", err)
	}
	if got := readTree(t, h.saves()); !mapsEqual(got, otherDevice) {
		t.Fatalf("saves = %v", got)
	}
	if err := h.svc.ServiceShutdown(); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
	if n := h.client.requestsFor(gameCloud) - transfers; n != 1 {
		t.Fatalf("%d transfers of %s, want only the pass's", n, gameCloud)
	}
}

// Case 4.
func TestLaunchStartsWhenTheCloudCannotAnswer(t *testing.T) {
	unreachable := fmt.Errorf("test cause unreachable: %w", &account.Error{Code: account.CodeNetwork})
	serverDown := fmt.Errorf("test cause server: %w", &account.Error{Code: account.CodeServer, Status: 500})
	cases := []struct {
		name  string
		arm   func(h *harness)
		cause error
	}{
		{"unreachable", func(h *harness) { h.client.listErr = unreachable }, unreachable},
		{"server error", func(h *harness) { h.client.listErr = serverDown }, serverDown},
		{"no answer within the limit", func(h *harness) { h.t.Cleanup(h.client.hold(listKey)) }, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t)
			h.newerInCloud()
			h.open()
			limitLaunch(h, shortLimit)
			logs := captureLogs(t)
			tc.arm(h)
			lists := h.client.requestsFor(listKey)
			before := h.stateBytes()

			err := h.starting(gameLocal)

			wantStarts(t, err)
			if h.client.requestsFor(listKey) == lists {
				t.Fatal("the hook never asked the cloud")
			}
			switch {
			case tc.cause != nil && err != nil:
				if !errors.Is(err, tc.cause) {
					t.Fatalf("SessionStarting = %v, want the cause %v kept for the library's log", err, tc.cause)
				}
			case tc.cause != nil:
				if !strings.Contains(logs.String(), "test cause") {
					t.Fatalf("log = %q, want the failure logged with its cause", logs.String())
				}
			case err == nil && logs.String() == "":
				t.Fatal("the hook gave up on the cloud and logged nothing")
			}
			if _, d := h.client.transfers(); d != 0 {
				t.Fatalf("%d downloads without an answer from the cloud", d)
			}
			if got := readTree(t, h.saves()); !mapsEqual(got, baseFiles) {
				t.Fatalf("saves changed to %v", got)
			}
			if got := h.stateBytes(); !bytes.Equal(got, before) {
				t.Fatalf("state file changed:\n%s", got)
			}
		})
	}
}

// Case 5.
func TestLaunchHookIsInertWhenCloudSavesDoNotApply(t *testing.T) {
	cases := []struct {
		name string
		game string
		arm  func(h *harness)
	}{
		{"cloud saves off", gameLocal, func(h *harness) {
			h.recordConflict()
			h.setEnabled(false)
		}},
		{"signed out", gameLocal, func(h *harness) {
			h.recordConflict()
			h.setToken("")
		}},
		{"excluded", gameLocal, func(h *harness) {
			writeTree(h.t, h.saves(), baseFiles)
			h.pass()
			if err := h.svc.DeleteRemote(gameLocal); err != nil {
				h.t.Fatalf("DeleteRemote: %v", err)
			}
			h.seedRemote(otherDevice, "other-device")
			h.drain()
		}},
		{"no cloud identity", "no-key", func(h *harness) {
			writeTree(h.t, h.addGame("no-key", ""), baseFiles)
			h.seedRemote(otherDevice, "other-device")
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t)
			h.open()
			limitLaunch(h, time.Minute)
			tc.arm(h)
			calls := h.client.calls()
			before := h.stateBytes()
			saves := readTree(t, h.lib.saves[tc.game])

			err := h.starting(tc.game)

			if err != nil {
				t.Fatalf("SessionStarting = %v, want nil: the game starts as it does today", err)
			}
			if n := h.client.calls() - calls; n != 0 {
				t.Fatalf("%d cloud requests, want none", n)
			}
			if got := readTree(t, h.lib.saves[tc.game]); !mapsEqual(got, saves) {
				t.Fatalf("saves changed to %v", got)
			}
			if got := h.stateBytes(); !bytes.Equal(got, before) {
				t.Fatalf("state file changed:\n%s", got)
			}
			if evs := h.drain(); len(evs) != 0 {
				t.Fatalf("events = %+v, want none", evs)
			}
		})
	}
}

// Case 6.
func TestCancelledLaunchStopsWaitingForTheCloud(t *testing.T) {
	h := newHarness(t)
	h.newerInCloud()
	h.open()
	limitLaunch(h, time.Hour)
	release := h.client.hold(gameCloud)
	t.Cleanup(release)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := h.startingAsync(ctx, gameLocal)
	h.client.waitHeld(t, gameCloud)

	cancel()
	err := await(t, done, "SessionStarting after the launch was cancelled")

	if err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("SessionStarting = %v, want nil or context.Canceled so the library ends it as a cancelled launch", err)
	}
	if got := readTree(t, h.saves()); !mapsEqual(got, baseFiles) {
		t.Fatalf("saves changed to %v", got)
	}
	if st := h.recorded(gameCloud); st.Conflict != nil || st.RemoteDigest != contractDigest(baseFiles) {
		t.Fatalf("recorded %+v, want the record untouched", st)
	}
}

func TestLaunchDuringResolveConflictNeverStartsOnSavesBeingReplaced(t *testing.T) {
	h := newHarness(t)
	remoteDigest := h.recordConflict()
	h.open()
	limitLaunch(h, shortLimit)
	transfers := h.client.requestsFor(gameCloud)
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
	if err := h.starting(gameLocal); err != nil {
		t.Fatalf("launch after the conflict was resolved = %v, want nil", err)
	}
	if got := readTree(t, h.saves()); !mapsEqual(got, otherDevice) {
		t.Fatalf("saves = %v, want the kept cloud copy", got)
	}
	if st := h.persisted(gameCloud); st.Conflict != nil || st.LocalDigest != remoteDigest {
		t.Fatalf("state file = %+v, want the conflict resolved", st)
	}
	if n := h.client.requestsFor(gameCloud) - transfers; n != 1 {
		t.Fatalf("%d transfers of %s, want only the resolution's", n, gameCloud)
	}
}

func TestLaunchDoesNotWaitBehindAPassOfAnotherGame(t *testing.T) {
	for _, tc := range []struct {
		name     string
		conflict bool
	}{{"saves in sync", false}, {"conflict recorded", true}} {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t)
			otherSaves := h.addGame("local-2", "cloud-2")
			writeTree(t, otherSaves, baseFiles)
			if tc.conflict {
				h.recordConflict()
			} else {
				writeTree(t, h.saves(), baseFiles)
				h.pass()
				h.drain()
			}
			h.open()
			limitLaunch(h, shortLimit)
			writeTree(t, otherSaves, changedBase)
			release := h.client.hold("cloud-2")
			passDone := make(chan struct{})
			go func() {
				defer close(passDone)
				h.svc.pass(context.Background(), map[string]struct{}{"local-2": {}})
			}()
			t.Cleanup(func() {
				release()
				<-passDone
			})
			h.client.waitHeld(t, "cloud-2")

			err := h.starting(gameLocal)

			if tc.conflict {
				wantBlock(t, err, wantConflictCode)
			} else if err != nil {
				t.Fatalf("SessionStarting = %v, want nil", err)
			}
			select {
			case <-passDone:
				t.Fatal("setup: the other game's transfer ended before the launch answered")
			default:
			}
			release()
			awaitClosed(t, passDone, "the pass of the other game")
			if st := h.recorded("cloud-2"); st.LocalDigest != contractDigest(changedBase) {
				t.Fatalf("the other game's pass did not finish its upload: %+v", st)
			}
		})
	}
}

func TestTwoLaunchesOfTheSameGameShareOneDownload(t *testing.T) {
	h := newHarness(t)
	remoteDigest := h.newerInCloud()
	h.open()
	limitLaunch(h, shortLimit)
	transfers := h.client.requestsFor(gameCloud)
	release := h.client.hold(gameCloud)
	t.Cleanup(release)

	first := h.startingAsync(context.Background(), gameLocal)
	second := h.startingAsync(context.Background(), gameLocal)
	wantBlock(t, await(t, first, "first SessionStarting"), wantSyncingCode)
	wantBlock(t, await(t, second, "second SessionStarting"), wantSyncingCode)
	h.client.waitHeld(t, gameCloud)
	if n := h.client.requestsFor(gameCloud) - transfers; n != 1 {
		t.Fatalf("%d downloads of one game in flight, want 1", n)
	}

	release()
	waitFor(t, "the background download to be recorded", func() bool {
		return h.recorded(gameCloud).RemoteDigest == remoteDigest
	})
	if err := h.starting(gameLocal); err != nil {
		t.Fatalf("launch after the download finished = %v, want nil", err)
	}
	if got := readTree(t, h.saves()); !mapsEqual(got, otherDevice) {
		t.Fatalf("saves = %v", got)
	}
	if err := h.svc.ServiceShutdown(); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
	if n := h.client.requestsFor(gameCloud) - transfers; n != 1 {
		t.Fatalf("%d downloads of one game, want 1", n)
	}
	if n := h.backups.restoreCount(); n != 1 {
		t.Fatalf("%d restores, want 1", n)
	}
}

func TestShutdownEndsTheLaunchDownloadAndLeavesNothingRunning(t *testing.T) {
	h := newHarness(t)
	h.newerInCloud()
	h.open()
	limitLaunch(h, shortLimit)
	release := h.client.hold(gameCloud)
	t.Cleanup(release)
	before := h.stateBytes()
	wantBlock(t, h.starting(gameLocal), wantSyncingCode)
	h.client.waitHeld(t, gameCloud)

	stopped := make(chan error, 1)
	go func() { stopped <- h.svc.ServiceShutdown() }()
	if err := await(t, stopped, "ServiceShutdown with a launch download in flight"); err != nil {
		t.Fatalf("shutdown: %v", err)
	}

	if n := h.client.inFlightNow(); n != 0 {
		t.Fatalf("%d cloud requests still running after shutdown returned: the launch download is not owned by the service", n)
	}
	if got := readTree(t, h.saves()); !mapsEqual(got, baseFiles) {
		t.Fatalf("saves changed to %v by a download cut off at shutdown", got)
	}
	if got := h.stateBytes(); !bytes.Equal(got, before) {
		t.Fatalf("state file changed by a download cut off at shutdown:\n%s", got)
	}
	entries, err := os.ReadDir(h.svc.store.tmpDir())
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("read temporary directory: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("temporary directory keeps %v after shutdown", entries)
	}
}

// Case 4 after the limit: the launch went ahead without an answer, so a later
// answer that shows a newer cloud copy must leave the saves of the game that
// is running by then alone.
func TestLaunchThatGaveUpLeavesTheSavesOfTheRunningGameAlone(t *testing.T) {
	h := newHarness(t)
	h.newerInCloud()
	h.open()
	limitLaunch(h, shortLimit)
	releaseList := h.client.hold(listKey)
	t.Cleanup(releaseList)
	before := h.stateBytes()
	recorded := h.recorded(gameCloud)

	wantStarts(t, h.starting(gameLocal))
	h.lib.setRunning(gameLocal, true)
	releaseList()
	waitFor(t, "the launch check to end", func() bool { return h.launchChecks() == 0 })

	if _, d := h.client.transfers(); d != 0 || h.backups.restoreCount() != 0 {
		t.Fatalf("%d downloads and %d restores over the saves of a running game", d, h.backups.restoreCount())
	}
	if got := readTree(t, h.saves()); !mapsEqual(got, baseFiles) {
		t.Fatalf("saves of the running game changed to %v", got)
	}
	if got := h.recorded(gameCloud); got != recorded {
		t.Fatalf("recorded %+v, want %+v untouched", got, recorded)
	}
	if got := h.stateBytes(); !bytes.Equal(got, before) {
		t.Fatalf("state file changed:\n%s", got)
	}
}

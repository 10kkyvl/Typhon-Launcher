package accountsync

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"typhon/internal/clientid"
	"typhon/internal/settings"

	"github.com/google/uuid"
)

func TestSyncDisabledMakesNoRequest(t *testing.T) {
	h := newHarness(t)
	h.settings.value.AccountSync = false
	h.server.get = func(w http.ResponseWriter) { t.Fatal("GET should not be called when account sync is disabled") }
	h.server.put = func(w http.ResponseWriter, req putRequest) {
		t.Fatal("PUT should not be called when account sync is disabled")
	}

	if err := h.service.Sync(context.Background()); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if h.server.getCalls != 0 || h.server.putCalls != 0 {
		t.Fatalf("expected no requests, got get=%d put=%d", h.server.getCalls, h.server.putCalls)
	}
}

func TestSyncFirstRunGeneratesDeviceID(t *testing.T) {
	h := newHarness(t)
	h.server.get = func(w http.ResponseWriter) {
		writeJSON(w, http.StatusOK, snapshotBody{Games: []wireGame{}})
	}
	h.server.put = echoPut(http.StatusOK)

	before := h.readState()
	if before.DeviceID != "" {
		t.Fatalf("expected empty device id before first sync, got %q", before.DeviceID)
	}

	if err := h.service.Sync(context.Background()); err != nil {
		t.Fatalf("Sync: %v", err)
	}

	after := h.readState()
	if after.DeviceID == "" {
		t.Fatal("expected a device id to be generated and saved")
	}
	if _, err := uuid.Parse(after.DeviceID); err != nil {
		t.Fatalf("device id is not a valid uuid: %v", err)
	}

	installPath := filepath.Join(h.dir, "installation.json")
	identity, err := clientid.LoadAt(installPath)
	if err != nil {
		t.Fatalf("clientid.LoadAt: %v", err)
	}
	if after.DeviceID == identity.InstallationID {
		t.Fatal("accountsync device id must not equal the pseudonymous installation id")
	}

	h.reopen()
	if err := h.service.Sync(context.Background()); err != nil {
		t.Fatalf("Sync after restart: %v", err)
	}
	afterRestart := h.readState()
	if afterRestart.DeviceID != after.DeviceID {
		t.Fatalf("device id changed across a service restart: before=%q after=%q", after.DeviceID, afterRestart.DeviceID)
	}
}

func TestSyncPlaytimeAccounting(t *testing.T) {
	h := newHarness(t)
	h.catalog.link("game-42", "42")
	h.library.setLocal("game-42", 100)

	var lastPut putRequest
	h.server.put = func(w http.ResponseWriter, req putRequest) {
		lastPut = req
		echoPut(http.StatusOK)(w, req)
	}

	t.Run("first sync pushes the local total (no prior baseline)", func(t *testing.T) {
		h.server.get = func(w http.ResponseWriter) {
			writeJSON(w, http.StatusOK, snapshotBody{Games: []wireGame{}})
		}
		if err := h.service.Sync(context.Background()); err != nil {
			t.Fatalf("Sync: %v", err)
		}
		g, ok := pushedFor(lastPut, 42)
		if !ok || g.PlaytimeSeconds != 100 {
			t.Fatalf("expected push of 100s for game 42, got %+v (ok=%v)", g, ok)
		}
	})

	t.Run("no local growth: repeat sync sends the same total, not doubled", func(t *testing.T) {
		h.server.get = func(w http.ResponseWriter) {
			writeJSON(w, http.StatusOK, snapshotBody{Games: []wireGame{{IGDBID: 42, PlaytimeSeconds: 100}}})
		}
		if err := h.service.Sync(context.Background()); err != nil {
			t.Fatalf("Sync: %v", err)
		}
		g, ok := pushedFor(lastPut, 42)
		if !ok || g.PlaytimeSeconds != 100 {
			t.Fatalf("expected push to stay at 100s, got %+v (ok=%v)", g, ok)
		}
	})

	t.Run("receiving another device's time updates local, but does not inflate this device's push", func(t *testing.T) {
		h.library.setLocal("game-42", 100)
		h.server.get = func(w http.ResponseWriter) {
			writeJSON(w, http.StatusOK, snapshotBody{Games: []wireGame{{IGDBID: 42, PlaytimeSeconds: 180}}})
		}
		if err := h.service.Sync(context.Background()); err != nil {
			t.Fatalf("Sync: %v", err)
		}
		if got := h.library.playtimeOf("game-42"); got != 180 {
			t.Fatalf("expected local playtime to become 180, got %d", got)
		}
		g, ok := pushedFor(lastPut, 42)
		if !ok || g.PlaytimeSeconds != 100 {
			t.Fatalf("expected this sync to still push only this device's 100s, got %+v (ok=%v)", g, ok)
		}
	})

	t.Run("new local session after receiving remote time pushes only the new delta", func(t *testing.T) {
		h.library.setLocal("game-42", 200)
		h.server.get = func(w http.ResponseWriter) {
			writeJSON(w, http.StatusOK, snapshotBody{Games: []wireGame{{IGDBID: 42, PlaytimeSeconds: 180}}})
		}
		if err := h.service.Sync(context.Background()); err != nil {
			t.Fatalf("Sync: %v", err)
		}
		g, ok := pushedFor(lastPut, 42)
		if !ok || g.PlaytimeSeconds != 120 {
			t.Fatalf("expected push of 120s (100 previous + 20 new), got %+v (ok=%v)", g, ok)
		}
	})
}

func TestSyncSettingsRevision(t *testing.T) {
	t.Run("unchanged revision: local settings win and are pushed", func(t *testing.T) {
		h := newHarness(t)
		h.settings.value.Theme = "dark"
		h.server.get = func(w http.ResponseWriter) {
			writeJSON(w, http.StatusOK, snapshotBody{SettingsRevision: 0, Games: []wireGame{}})
		}
		var pushedTheme *string
		h.server.put = func(w http.ResponseWriter, req putRequest) {
			if req.Settings != nil {
				pushedTheme = req.Settings.Theme
			}
			echoPut(http.StatusOK)(w, req)
		}

		if err := h.service.Sync(context.Background()); err != nil {
			t.Fatalf("Sync: %v", err)
		}
		if h.settings.saveCalls != 0 {
			t.Fatalf("expected no settings save when revision is unchanged, got %d calls", h.settings.saveCalls)
		}
		if h.settings.value.Theme != "dark" {
			t.Fatalf("expected local theme to survive, got %q", h.settings.value.Theme)
		}
		if pushedTheme == nil || *pushedTheme != "dark" {
			t.Fatalf("expected local theme to be pushed, got %v", pushedTheme)
		}
	})

	t.Run("changed revision: remote settings are applied", func(t *testing.T) {
		h := newHarness(t)
		h.settings.value.Theme = "dark"
		light := "light"
		h.server.get = func(w http.ResponseWriter) {
			writeJSON(w, http.StatusOK, snapshotBody{
				SettingsRevision: 7,
				Settings:         settings.Portable{Theme: &light},
				Games:            []wireGame{},
			})
		}
		h.server.put = echoPut(http.StatusOK)

		if err := h.service.Sync(context.Background()); err != nil {
			t.Fatalf("Sync: %v", err)
		}
		if h.settings.value.Theme != "light" {
			t.Fatalf("expected remote theme to be applied, got %q", h.settings.value.Theme)
		}
		if h.settings.saveCalls != 1 {
			t.Fatalf("expected exactly one settings save, got %d", h.settings.saveCalls)
		}
	})
}

func TestSyncConflictRetriesOnce(t *testing.T) {
	t.Run("recovers after a single conflict", func(t *testing.T) {
		h := newHarness(t)
		h.server.get = func(w http.ResponseWriter) {
			writeJSON(w, http.StatusOK, snapshotBody{SettingsRevision: 0, Games: []wireGame{}})
		}
		attempt := 0
		h.server.put = func(w http.ResponseWriter, req putRequest) {
			attempt++
			if attempt == 1 {
				writeSyncError(w, http.StatusConflict, "sync_conflict", "settingsRevision")
				return
			}
			echoPut(http.StatusOK)(w, req)
		}

		if err := h.service.Sync(context.Background()); err != nil {
			t.Fatalf("Sync: %v", err)
		}
		if h.server.getCalls != 2 {
			t.Fatalf("expected the snapshot to be refetched once after a conflict, got %d GETs", h.server.getCalls)
		}
		if attempt != 2 {
			t.Fatalf("expected exactly one retry, got %d PUT attempts", attempt)
		}
	})

	t.Run("returns an error after a second conflict", func(t *testing.T) {
		h := newHarness(t)
		h.server.get = func(w http.ResponseWriter) {
			writeJSON(w, http.StatusOK, snapshotBody{SettingsRevision: 0, Games: []wireGame{}})
		}
		h.server.put = func(w http.ResponseWriter, req putRequest) {
			writeSyncError(w, http.StatusConflict, "sync_conflict", "settingsRevision")
		}

		err := h.service.Sync(context.Background())
		if !errors.Is(err, ErrConflict) {
			t.Fatalf("expected ErrConflict, got %v", err)
		}
		if h.server.putCalls != 2 {
			t.Fatalf("expected exactly two PUT attempts (initial + one retry), got %d", h.server.putCalls)
		}
	})
}

func TestSyncAuthAndNetworkErrorsAreDistinctAndSafe(t *testing.T) {
	t.Run("401 surfaces as ErrUnauthorized and leaves state untouched", func(t *testing.T) {
		h := newHarness(t)
		h.server.get = func(w http.ResponseWriter) {
			writeSyncError(w, http.StatusUnauthorized, "unauthenticated", "")
		}

		before := h.readState()
		err := h.service.Sync(context.Background())
		if !errors.Is(err, ErrUnauthorized) {
			t.Fatalf("expected ErrUnauthorized, got %v", err)
		}
		after := h.readState()
		if !stateEqual(before, after) {
			t.Fatalf("state changed after a failed sync: before=%+v after=%+v", before, after)
		}
	})

	t.Run("network failure is a distinct, non-nil error and leaves state untouched", func(t *testing.T) {
		h := newHarness(t)
		h.server.Close()

		before := h.readState()
		err := h.service.Sync(context.Background())
		var netErr *NetworkError
		if !errors.As(err, &netErr) {
			t.Fatalf("expected *NetworkError, got %T: %v", err, err)
		}
		if errors.Is(err, ErrUnauthorized) {
			t.Fatal("network error must not be classified as unauthorized")
		}
		after := h.readState()
		if !stateEqual(before, after) {
			t.Fatalf("state changed after a failed sync: before=%+v after=%+v", before, after)
		}
	})
}

func TestSyncPutFailureLeavesStateUnchanged(t *testing.T) {
	h := newHarness(t)
	h.catalog.link("game-42", "42")
	h.library.setLocal("game-42", 100)
	h.server.get = func(w http.ResponseWriter) {
		writeJSON(w, http.StatusOK, snapshotBody{Games: []wireGame{}})
	}
	h.server.put = func(w http.ResponseWriter, req putRequest) {
		w.WriteHeader(http.StatusInternalServerError)
	}

	before := h.readState()
	err := h.service.Sync(context.Background())
	var serverErr *ServerError
	if !errors.As(err, &serverErr) {
		t.Fatalf("expected *ServerError, got %T: %v", err, err)
	}

	after := h.readState()
	if after.DeviceID != before.DeviceID {
		t.Fatalf("device id changed on disk despite a failed push: before=%q after=%q", before.DeviceID, after.DeviceID)
	}
	if len(after.Games) != len(before.Games) {
		t.Fatalf("game state changed on disk despite a failed push: before=%+v after=%+v", before.Games, after.Games)
	}
}

func TestSyncChunksMoreThan500Games(t *testing.T) {
	h := newHarness(t)
	const total = 650
	for i := 0; i < total; i++ {
		id := strconv.Itoa(i)
		canonical := "game-" + id
		h.catalog.link(canonical, id)
		h.library.setLocal(canonical, 10)
	}
	h.server.get = func(w http.ResponseWriter) {
		writeJSON(w, http.StatusOK, snapshotBody{Games: []wireGame{}})
	}
	h.server.put = echoPut(http.StatusOK)

	if err := h.service.Sync(context.Background()); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if h.server.putCalls != 2 {
		t.Fatalf("expected 2 PUT requests for %d games, got %d", total, h.server.putCalls)
	}
	settingsCarriers := 0
	pushedGames := 0
	for _, body := range h.server.putBodies {
		if body.Settings != nil {
			settingsCarriers++
		}
		pushedGames += len(body.Games)
	}
	if settingsCarriers != 1 {
		t.Fatalf("expected settings to be sent exactly once across chunks, got %d", settingsCarriers)
	}
	if pushedGames != total {
		t.Fatalf("expected %d games pushed across chunks, got %d", total, pushedGames)
	}
}

func TestSyncHydrationCap(t *testing.T) {
	h := newHarness(t)
	const remoteNew = 25
	remote := make([]wireGame, remoteNew)
	for i := 0; i < remoteNew; i++ {
		remote[i] = wireGame{IGDBID: int64(1000 + i), PlaytimeSeconds: 5}
	}
	h.server.get = func(w http.ResponseWriter) {
		writeJSON(w, http.StatusOK, snapshotBody{Games: remote})
	}
	h.server.put = echoPut(http.StatusOK)

	if err := h.service.Sync(context.Background()); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if len(h.library.addCalls) != maxHydratePerSync {
		t.Fatalf("expected exactly %d games hydrated in one sync, got %d", maxHydratePerSync, len(h.library.addCalls))
	}

	if err := h.service.Sync(context.Background()); err != nil {
		t.Fatalf("second Sync: %v", err)
	}
	if len(h.library.addCalls) != remoteNew {
		t.Fatalf("expected the remaining games to be hydrated on the next cycle: got %d of %d total", len(h.library.addCalls), remoteNew)
	}
}

func TestSyncMetadataErrorDoesNotAbortOthers(t *testing.T) {
	h := newHarness(t)
	h.metadata.titleErr["2"] = errors.New("metadata unavailable")
	h.server.get = func(w http.ResponseWriter) {
		writeJSON(w, http.StatusOK, snapshotBody{Games: []wireGame{
			{IGDBID: 1, PlaytimeSeconds: 1},
			{IGDBID: 2, PlaytimeSeconds: 1},
			{IGDBID: 3, PlaytimeSeconds: 1},
		}})
	}
	h.server.put = echoPut(http.StatusOK)

	if err := h.service.Sync(context.Background()); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if len(h.library.addCalls) != 2 {
		t.Fatalf("expected the 2 healthy games to be hydrated despite the failure, got %d: %v", len(h.library.addCalls), h.library.addCalls)
	}
	for _, canonical := range h.library.addCalls {
		if canonical == "game-2" {
			t.Fatal("game 2 should not have been hydrated: its metadata lookup failed")
		}
	}
}

func TestNewServiceRejectsCorruptState(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "sync.json"), []byte("{not json"), 0o600); err != nil {
		t.Fatalf("seed corrupt state: %v", err)
	}
	syncSettings := settings.Defaults()
	syncSettings.AccountSync = true
	h := &harness{
		t:        t,
		dir:      dir,
		settings: &fakeSettings{value: syncSettings},
		library:  newFakeLibrary(),
		catalog:  newFakeCatalog(),
		metadata: newFakeMetadata(),
	}
	_, err := NewService(dir, "https://example.invalid", testToken, h.settings, h.library, h.catalog, h.metadata)
	if err == nil {
		t.Fatal("expected NewService to fail on corrupt state instead of starting empty")
	}
}

func TestSyncConcurrentCallsAreSerialized(t *testing.T) {
	h := newHarness(t)
	h.server.get = func(w http.ResponseWriter) {
		writeJSON(w, http.StatusOK, snapshotBody{Games: []wireGame{}})
	}
	entered := make(chan struct{})
	release := make(chan struct{})
	h.server.put = func(w http.ResponseWriter, req putRequest) {
		close(entered)
		<-release
		echoPut(http.StatusOK)(w, req)
	}

	var firstErr error
	done := make(chan struct{})
	go func() {
		firstErr = h.service.Sync(context.Background())
		close(done)
	}()

	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("first sync never reached the server")
	}

	secondErr := h.service.Sync(context.Background())
	if !errors.Is(secondErr, ErrSyncInProgress) {
		t.Fatalf("expected ErrSyncInProgress for the overlapping call, got %v", secondErr)
	}

	close(release)
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("first sync never completed")
	}
	if firstErr != nil {
		t.Fatalf("first sync failed: %v", firstErr)
	}
}

func TestSyncCancelledContext(t *testing.T) {
	h := newHarness(t)
	h.server.get = func(w http.ResponseWriter) { t.Fatal("GET should not be called with an already-cancelled context") }
	h.server.put = func(w http.ResponseWriter, req putRequest) {
		t.Fatal("PUT should not be called with an already-cancelled context")
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := h.service.Sync(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	if h.server.getCalls != 0 {
		t.Fatalf("expected no requests with a cancelled context, got %d GETs", h.server.getCalls)
	}
}

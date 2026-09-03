package accountsync

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"typhon/internal/settings"
)

type fakeSettings struct {
	mu        sync.Mutex
	value     settings.Settings
	saveErr   error
	saveCalls int
}

func (f *fakeSettings) Get() settings.Settings {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.value
}

func (f *fakeSettings) Save(s settings.Settings) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.saveErr != nil {
		return f.saveErr
	}
	f.value = s
	f.saveCalls++
	return nil
}

type fakeLibrary struct {
	mu          sync.Mutex
	games       map[string]Game
	snapshotErr error
	applyErr    error
	addErr      map[string]error
	addCalls    []string
	applyCalls  int
}

func newFakeLibrary() *fakeLibrary {
	return &fakeLibrary{games: map[string]Game{}, addErr: map[string]error{}}
}

func (f *fakeLibrary) Snapshot() ([]Game, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.snapshotErr != nil {
		return nil, f.snapshotErr
	}
	out := make([]Game, 0, len(f.games))
	for _, g := range f.games {
		out = append(out, g)
	}
	return out, nil
}

func (f *fakeLibrary) Apply(items []Game) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.applyErr != nil {
		return f.applyErr
	}
	f.applyCalls++
	for _, it := range items {
		f.games[it.CanonicalGameID] = Game{
			CanonicalGameID: it.CanonicalGameID,
			PlaytimeSeconds: it.PlaytimeSeconds,
			Owned:           it.Owned,
			LastPlayed:      it.LastPlayed,
			Favorite:        it.Favorite,
			Status:          it.Status,
			StatusAt:        it.StatusAt,
		}
	}
	return nil
}

func (f *fakeLibrary) Add(canonicalGameID, title string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.addCalls = append(f.addCalls, canonicalGameID)
	if err, ok := f.addErr[canonicalGameID]; ok {
		return err
	}
	f.games[canonicalGameID] = Game{CanonicalGameID: canonicalGameID}
	return nil
}

func (f *fakeLibrary) setLocal(canonicalID string, seconds int64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.games[canonicalID] = Game{CanonicalGameID: canonicalID, PlaytimeSeconds: seconds}
}

func (f *fakeLibrary) setLocalGame(g Game) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.games[g.CanonicalGameID] = g
}

func (f *fakeLibrary) gameOf(canonicalID string) Game {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.games[canonicalID]
}

func (f *fakeLibrary) playtimeOf(canonicalID string) int64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.games[canonicalID].PlaytimeSeconds
}

type fakeCatalog struct {
	mu          sync.Mutex
	byCanonical map[string]string
	ensureErr   map[string]error
}

func newFakeCatalog() *fakeCatalog {
	return &fakeCatalog{byCanonical: map[string]string{}, ensureErr: map[string]error{}}
}

func (f *fakeCatalog) link(canonicalID, igdbID string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.byCanonical[canonicalID] = igdbID
}

func (f *fakeCatalog) IGDBIDOf(canonicalGameID string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.byCanonical[canonicalGameID]
}

func (f *fakeCatalog) EnsureByIGDB(igdbID, title string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err, ok := f.ensureErr[igdbID]; ok {
		return "", err
	}
	canonical := "game-" + igdbID
	f.byCanonical[canonical] = igdbID
	return canonical, nil
}

type fakeMetadata struct {
	mu       sync.Mutex
	titleErr map[string]error
	calls    []string
}

func newFakeMetadata() *fakeMetadata {
	return &fakeMetadata{titleErr: map[string]error{}}
}

func (f *fakeMetadata) Title(ctx context.Context, igdbID string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, igdbID)
	if err, ok := f.titleErr[igdbID]; ok {
		return "", err
	}
	return "Title " + igdbID, nil
}

type mockSyncServer struct {
	*httptest.Server
	mu        sync.Mutex
	getCalls  int
	putCalls  int
	delCalls  int
	putBodies []putRequest
	get       func(w http.ResponseWriter)
	put       func(w http.ResponseWriter, req putRequest)
	del       func(w http.ResponseWriter)
}

func newMockSyncServer(t *testing.T) *mockSyncServer {
	m := &mockSyncServer{}
	m.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			m.mu.Lock()
			m.getCalls++
			fn := m.get
			m.mu.Unlock()
			if fn == nil {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			fn(w)
		case http.MethodPut:
			var req putRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			m.mu.Lock()
			m.putCalls++
			m.putBodies = append(m.putBodies, req)
			fn := m.put
			m.mu.Unlock()
			if fn == nil {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			fn(w, req)
		case http.MethodDelete:
			m.mu.Lock()
			m.delCalls++
			fn := m.del
			m.mu.Unlock()
			if fn == nil {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			fn(w)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	t.Cleanup(m.Close)
	return m
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		panic(fmt.Sprintf("test sync server: encode response: %v", err))
	}
}

func writeSyncError(w http.ResponseWriter, status int, code, field string) {
	writeJSON(w, status, map[string]any{"error": map[string]any{"code": code, "field": field}})
}

func echoPut(status int) func(w http.ResponseWriter, req putRequest) {
	return func(w http.ResponseWriter, req putRequest) {
		writeJSON(w, status, applyBody{
			snapshotBody: snapshotBody{
				SettingsRevision: req.SettingsRevision,
				Games:            req.Games,
			},
		})
	}
}

type harness struct {
	t        *testing.T
	dir      string
	server   *mockSyncServer
	settings *fakeSettings
	library  *fakeLibrary
	catalog  *fakeCatalog
	metadata *fakeMetadata
	service  *Service
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	dir := t.TempDir()
	server := newMockSyncServer(t)
	h := &harness{
		t:        t,
		dir:      dir,
		server:   server,
		settings: &fakeSettings{value: settings.Defaults()},
		library:  newFakeLibrary(),
		catalog:  newFakeCatalog(),
		metadata: newFakeMetadata(),
	}
	h.settings.value.AccountSync = true

	svc, err := NewService(dir, server.URL, testToken, h.settings, h.library, h.catalog, h.metadata)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	h.service = svc
	return h
}

func (h *harness) reopen() {
	h.t.Helper()
	svc, err := NewService(h.dir, h.server.URL, testToken, h.settings, h.library, h.catalog, h.metadata)
	if err != nil {
		h.t.Fatalf("NewService (reopen): %v", err)
	}
	h.service = svc
}

func (h *harness) readState() syncState {
	h.t.Helper()
	st, err := newStore(h.dir).load()
	if err != nil {
		h.t.Fatalf("read state: %v", err)
	}
	return st
}

func stateEqual(a, b syncState) bool {
	if a.DeviceID != b.DeviceID || a.SettingsRevision != b.SettingsRevision {
		return false
	}
	if len(a.Games) != len(b.Games) {
		return false
	}
	for id, g := range a.Games {
		if b.Games[id] != g {
			return false
		}
	}
	return true
}

func pushedFor(req putRequest, igdbID int64) (wireGame, bool) {
	for _, g := range req.Games {
		if g.IGDBID == igdbID {
			return g, true
		}
	}
	return wireGame{}, false
}

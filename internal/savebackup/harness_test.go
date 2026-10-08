package savebackup

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"typhon/internal/library"
	"typhon/internal/settings"

	"github.com/wailsapp/wails/v3/pkg/application"
)

type fakeGames struct {
	mu      sync.Mutex
	games   map[string]library.Game
	results map[string]library.SavesResult
	err     error
	running map[string]bool
	gate    chan struct{}
}

func (f *fakeGames) Find(id string) (library.Game, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	game, ok := f.games[id]
	if !ok {
		return library.Game{}, errors.New("game not found")
	}
	return game, nil
}

func (f *fakeGames) LocateSaves(ctx context.Context, id string) (library.SavesResult, error) {
	f.mu.Lock()
	gate, err, res := f.gate, f.err, f.results[id]
	f.mu.Unlock()
	if gate != nil {
		select {
		case <-gate:
		case <-ctx.Done():
			return library.SavesResult{}, ctx.Err()
		}
	}
	if err != nil {
		return library.SavesResult{}, err
	}
	return res, nil
}

func (f *fakeGames) GetRunningGames() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var ids []string
	for id, on := range f.running {
		if on {
			ids = append(ids, id)
		}
	}
	return ids
}

func (f *fakeGames) setRunning(id string, on bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.running[id] = on
}

func (f *fakeGames) setResult(id string, res library.SavesResult) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.results[id] = res
}

func (f *fakeGames) setSavesDir(id, dir string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	game := f.games[id]
	game.SavesDir = dir
	f.games[id] = game
}

const gameID = "g1"

type harness struct {
	t      *testing.T
	dir    string
	saves  string
	games  *fakeGames
	svc    *Service
	events chan Event
	cancel context.CancelFunc

	mu    sync.Mutex
	limit int
	after bool
	clock time.Time
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	h := &harness{
		t:      t,
		dir:    t.TempDir(),
		saves:  filepath.Join(t.TempDir(), "Saves"),
		events: make(chan Event, 1024),
		limit:  settings.DefaultSaveBackupLimit,
		after:  true,
		clock:  time.Date(2026, 9, 29, 15, 30, 0, 0, time.UTC),
	}
	writeFile(t, filepath.Join(h.saves, "slot1.sav"), "one")
	writeFile(t, filepath.Join(h.saves, "profile", "slot2.sav"), "two")
	h.games = &fakeGames{
		games:   map[string]library.Game{gameID: {ID: gameID, Title: "Game"}, "g2": {ID: "g2", Title: "Other"}},
		results: map[string]library.SavesResult{gameID: {Path: h.saves}},
		running: map[string]bool{},
	}
	h.start()
	return h
}

func (h *harness) config() settings.Settings {
	h.mu.Lock()
	defer h.mu.Unlock()
	cfg := settings.Defaults()
	cfg.SaveBackupLimit = h.limit
	cfg.SaveBackupAfterSession = h.after
	return cfg
}

func (h *harness) setLimit(n int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.limit = n
}

func (h *harness) setAfter(on bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.after = on
}

func (h *harness) tick() time.Time {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.clock = h.clock.Add(time.Second)
	return h.clock
}

func (h *harness) build() *Service {
	h.t.Helper()
	svc, err := newServiceAt(h.dir, h.config, h.games)
	if err != nil {
		h.t.Fatalf("new service: %v", err)
	}
	svc.now = h.tick
	svc.emit = func(ev Event) { h.events <- ev }
	return svc
}

func (h *harness) start() {
	h.t.Helper()
	h.svc = h.build()
	ctx, cancel := context.WithCancel(context.Background())
	h.cancel = cancel
	if err := h.svc.ServiceStartup(ctx, application.ServiceOptions{}); err != nil {
		h.t.Fatalf("startup: %v", err)
	}
	h.svc.wg.Wait()
	svc := h.svc
	h.t.Cleanup(func() {
		cancel()
		if err := svc.ServiceShutdown(); err != nil {
			h.t.Errorf("shutdown: %v", err)
		}
	})
}

func (h *harness) restart() {
	h.t.Helper()
	h.cancel()
	if err := h.svc.ServiceShutdown(); err != nil {
		h.t.Fatalf("shutdown: %v", err)
	}
	h.start()
}

func (h *harness) gameDir() string {
	return filepath.Join(h.dir, rootDirName, gameID)
}

func (h *harness) create() Snapshot {
	h.t.Helper()
	snap, err := h.svc.Create(context.Background(), gameID)
	if err != nil {
		h.t.Fatalf("create: %v", err)
	}
	return snap
}

func (h *harness) list() []Snapshot {
	h.t.Helper()
	list, err := h.svc.List(context.Background(), gameID)
	if err != nil {
		h.t.Fatalf("list: %v", err)
	}
	return list
}

func (h *harness) drain() []Event {
	var out []Event
	for {
		select {
		case ev := <-h.events:
			out = append(out, ev)
		default:
			return out
		}
	}
}

func (h *harness) next() Event {
	h.t.Helper()
	select {
	case ev := <-h.events:
		return ev
	case <-time.After(10 * time.Second):
		h.t.Fatal("no saves:backups event arrived")
		return Event{}
	}
}

func writeFile(t testing.TB, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readFile(t testing.TB, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func exists(t testing.TB, path string) bool {
	t.Helper()
	_, err := os.Lstat(path)
	if err == nil {
		return true
	}
	if errors.Is(err, fs.ErrNotExist) {
		return false
	}
	t.Fatal(err)
	return false
}

func tree(t testing.TB, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	fsys := os.DirFS(root)
	err := fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		data, err := fs.ReadFile(fsys, path)
		if err != nil {
			return err
		}
		out[path] = string(data)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func sameTree(t testing.TB, got, want map[string]string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("tree = %v, want %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Fatalf("tree = %v, want %v", got, want)
		}
	}
}

func snapshotsOfKind(list []Snapshot, kind Kind) []Snapshot {
	var out []Snapshot
	for _, s := range list {
		if s.Kind == kind {
			out = append(out, s)
		}
	}
	return out
}

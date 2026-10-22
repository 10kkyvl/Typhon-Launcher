package achievements

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"typhon/internal/uierr"
)

type fakeLookup struct {
	games map[string]GameRef
	err   error
}

func (f fakeLookup) Lookup(id string) (GameRef, bool, error) {
	if f.err != nil {
		return GameRef{}, false, f.err
	}
	ref, ok := f.games[id]
	return ref, ok, nil
}

type fakeSchema struct {
	mu    sync.Mutex
	items []SchemaItem
	err   error
	calls int
	langs []string
}

func (f *fakeSchema) Schema(_ context.Context, _, lang string) ([]SchemaItem, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.langs = append(f.langs, lang)
	return slices.Clone(f.items), f.err
}

func (f *fakeSchema) set(items []SchemaItem, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.items, f.err = items, err
}

func (f *fakeSchema) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

type clock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *clock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

type events struct {
	mu  sync.Mutex
	all []Event
}

func (e *events) emit(ev Event) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.all = append(e.all, ev)
}

func (e *events) snapshot() []Event {
	e.mu.Lock()
	defer e.mu.Unlock()
	return slices.Clone(e.all)
}

type harness struct {
	svc       *Service
	schema    *fakeSchema
	clock     *clock
	events    *events
	configDir string
	install   string
	lang      string
}

var testSchema = []SchemaItem{
	{Key: "ACH_A", Name: "Alpha", Description: "first", Icon: "https://cdn.example/a.jpg"},
	{Key: "ACH_B", Name: "Beta", Description: "second", Hidden: true},
	{Key: "ACH_C", Name: "Gamma"},
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	h := &harness{
		schema:    &fakeSchema{items: testSchema},
		clock:     &clock{now: time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)},
		events:    &events{},
		configDir: t.TempDir(),
		install:   filepath.Join(t.TempDir(), "game"),
		lang:      "ru",
	}
	svc, err := New(Config{
		ConfigDir: h.configDir,
		Games: fakeLookup{games: map[string]GameRef{
			"g1":       {InstallDir: h.install, AppID: "620"},
			"no-steam": {InstallDir: h.install},
		}},
		Schema:   h.schema,
		Clock:    h.clock.Now,
		Language: func() string { return h.lang },
	})
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	svc.scanner = scanner{getenv: func(string) string { return "" }, ls: os.ReadDir}
	svc.emit = h.events.emit
	h.svc = svc
	return h
}

func (h *harness) writeProgress(t *testing.T, body string) {
	t.Helper()
	writeFile(t, filepath.Join(h.install, "620", "achievements.json"), body)
}

func keysOf(view View) map[string]Item {
	out := map[string]Item{}
	for _, it := range view.Items {
		out[it.Key] = it
	}
	return out
}

func TestNewValidatesConfig(t *testing.T) {
	ok := Config{ConfigDir: t.TempDir(), Games: fakeLookup{}, Schema: &fakeSchema{}, Language: func() string { return "en" }}
	cases := []struct {
		name   string
		mutate func(*Config)
	}{
		{"empty config dir", func(c *Config) { c.ConfigDir = "" }},
		{"relative config dir", func(c *Config) { c.ConfigDir = "state" }},
		{"no lookup", func(c *Config) { c.Games = nil }},
		{"no schema source", func(c *Config) { c.Schema = nil }},
		{"no language", func(c *Config) { c.Language = nil }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := ok
			tc.mutate(&cfg)
			if svc, err := New(cfg); err == nil || svc != nil {
				t.Fatalf("svc %v err %v, want a constructor error", svc, err)
			}
		})
	}
	if _, err := New(ok); err != nil {
		t.Fatalf("valid config: %v", err)
	}
}

func TestScanStoresUnlocksAndKeepsThemWhenTheFileVanishes(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.writeProgress(t, `{"ACH_A":{"earned":true,"earned_time":1700000000}}`)

	prog, err := h.svc.Scan(ctx, "g1")
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if prog.Unlocked != 1 || !slices.Equal(prog.NewKeys, []string{"ACH_A"}) || !prog.Found {
		t.Fatalf("progress %+v", prog)
	}

	if err := os.Remove(filepath.Join(h.install, "620", "achievements.json")); err != nil {
		t.Fatal(err)
	}
	prog, err = h.svc.Scan(ctx, "g1")
	if err != nil {
		t.Fatalf("scan after the file vanished: %v", err)
	}
	if prog.Unlocked != 1 || len(prog.NewKeys) != 0 {
		t.Fatalf("an unlocked achievement must not lock again: %+v", prog)
	}
	view, err := h.svc.GameAchievements(ctx, "g1")
	if err != nil {
		t.Fatalf("view: %v", err)
	}
	if got := keysOf(view)["ACH_A"]; !got.Unlocked || got.UnlockedAt != "2023-11-14T22:13:20Z" {
		t.Fatalf("ACH_A = %+v", got)
	}
}

func TestScanAnnouncesOnlyNewUnlocks(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.writeProgress(t, `{"ACH_A":{"earned":true}}`)
	if _, err := h.svc.GameAchievements(ctx, "g1"); err != nil {
		t.Fatalf("first look: %v", err)
	}
	if got := h.events.snapshot(); len(got) != 0 {
		t.Fatalf("the first look must not announce what was earned long ago: %+v", got)
	}

	h.writeProgress(t, `{"ACH_A":{"earned":true},"ACH_B":{"earned":true},"ACH_NEW":{"earned":true}}`)
	if _, err := h.svc.Scan(ctx, "g1"); err != nil {
		t.Fatalf("scan: %v", err)
	}
	got := h.events.snapshot()
	if len(got) != 1 || got[0].GameID != "g1" {
		t.Fatalf("events %+v", got)
	}
	if !slices.Equal(got[0].Keys, []string{"ACH_B", "ACH_NEW"}) || !slices.Equal(got[0].Names, []string{"Beta", "ACH_NEW"}) {
		t.Fatalf("event %+v: names follow the schema and fall back to the key", got[0])
	}

	if _, err := h.svc.Scan(ctx, "g1"); err != nil {
		t.Fatalf("repeat scan: %v", err)
	}
	if n := len(h.events.snapshot()); n != 1 {
		t.Fatalf("a repeated scan announced again: %d events", n)
	}
}

func TestFirstLookIsABaselineForEveryTrigger(t *testing.T) {
	triggers := map[string]func(h *harness) error{
		"scan": func(h *harness) error {
			_, err := h.svc.Scan(context.Background(), "g1")
			return err
		},
		"rescan": func(h *harness) error {
			_, err := h.svc.Rescan(context.Background(), "g1")
			return err
		},
		"view": func(h *harness) error {
			_, err := h.svc.GameAchievements(context.Background(), "g1")
			return err
		},
	}
	for name, trigger := range triggers {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t)
			h.writeProgress(t, `{"ACH_A":{"earned":true}}`)
			if err := trigger(h); err != nil {
				t.Fatalf("first look: %v", err)
			}
			if got := h.events.snapshot(); len(got) != 0 {
				t.Fatalf("the first look announced what was earned long ago: %+v", got)
			}
			if rec, ok := h.svc.record("g1"); !ok || !rec.Found || len(rec.Unlocked) != 1 {
				t.Fatalf("the baseline was not stored: %+v %v", rec, ok)
			}

			h.writeProgress(t, `{"ACH_A":{"earned":true},"ACH_B":{"earned":true}}`)
			if err := trigger(h); err != nil {
				t.Fatalf("second look: %v", err)
			}
			if name == "view" {
				if _, err := h.svc.Scan(context.Background(), "g1"); err != nil {
					t.Fatalf("scan: %v", err)
				}
			}
			got := h.events.snapshot()
			if len(got) != 1 || !slices.Equal(got[0].Keys, []string{"ACH_B"}) {
				t.Fatalf("only the key missing from the record may be announced: %+v", got)
			}
		})
	}
}

func TestScanErrors(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	if _, err := h.svc.Scan(ctx, "no-steam"); !errors.Is(err, ErrNoSteamID) || uierr.Code(err) != codeNoSteamID {
		t.Fatalf("no appid: %v", err)
	}
	if _, err := h.svc.Scan(ctx, "missing"); !errors.Is(err, ErrGameNotFound) {
		t.Fatalf("missing game: %v", err)
	}
	lookupErr := errors.New("library is locked")
	h.svc.games = fakeLookup{err: lookupErr}
	if _, err := h.svc.Scan(ctx, "g1"); !errors.Is(err, lookupErr) || errors.Is(err, ErrGameNotFound) {
		t.Fatalf("lookup failure must not read as a missing game: %v", err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	h.svc.games = fakeLookup{games: map[string]GameRef{"g1": {InstallDir: h.install, AppID: "620"}}}
	if _, err := h.svc.Scan(cancelled, "g1"); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled: %v", err)
	}
}

func TestGameAchievementsWithoutSteamID(t *testing.T) {
	h := newHarness(t)
	view, err := h.svc.GameAchievements(context.Background(), "no-steam")
	if !errors.Is(err, ErrNoSteamID) || view.Total != 0 || view.Source != SourceNone {
		t.Fatalf("view %+v err %v", view, err)
	}
	if h.schema.count() != 0 {
		t.Fatal("a game without an app id must not reach the server")
	}
}

func TestGameAchievementsKeepsLocalProgressWhenTheSchemaIsDown(t *testing.T) {
	h := newHarness(t)
	h.schema.set(nil, fmt.Errorf("%w: HTTP 503", ErrSchemaDown))
	h.writeProgress(t, `{"ACH_A":{"earned":true,"earned_time":1700000000},"ACH_B":{"earned":true}}`)

	view, err := h.svc.GameAchievements(context.Background(), "g1")
	if err != nil {
		t.Fatalf("local progress must not be lost to a schema failure: %v", err)
	}
	if view.SchemaError != SchemaUnavailable {
		t.Fatalf("schema error = %q", view.SchemaError)
	}
	if view.Source != SourceLocal || view.AppID != "620" || view.Total != 2 || view.Unlocked != 2 {
		t.Fatalf("view %+v", view)
	}
	for _, it := range view.Items {
		if it.Name != "" || !it.Unlocked {
			t.Fatalf("items must carry keys without names: %+v", it)
		}
	}
}

func TestGameAchievementsWithoutProgressSurfacesASchemaFailure(t *testing.T) {
	h := newHarness(t)
	h.schema.set(nil, fmt.Errorf("%w: HTTP 503", ErrSchemaDown))
	if _, err := h.svc.GameAchievements(context.Background(), "g1"); !errors.Is(err, ErrSchemaDown) {
		t.Fatalf("err = %v, nothing to show must be an error, not an empty list", err)
	}
}

func TestGameAchievementsGameWithoutAchievements(t *testing.T) {
	h := newHarness(t)
	h.schema.set(nil, fmt.Errorf("%w: HTTP 404", ErrNoSchema))
	view, err := h.svc.GameAchievements(context.Background(), "g1")
	if err != nil {
		t.Fatalf("view: %v", err)
	}
	if view.Total != 0 || view.Source != SourceNone || view.SchemaError != "" || view.Items == nil {
		t.Fatalf("view %+v", view)
	}
}

func TestGameAchievementsNoServerListWithLocalProgress(t *testing.T) {
	h := newHarness(t)
	h.schema.set(nil, fmt.Errorf("%w: HTTP 404", ErrNoSchema))
	h.writeProgress(t, `{"ACH_A":{"earned":true}}`)
	view, err := h.svc.GameAchievements(context.Background(), "g1")
	if err != nil {
		t.Fatalf("view: %v", err)
	}
	if view.SchemaError != SchemaNone || view.Source != SourceLocal || view.Total != 1 || view.Unlocked != 1 {
		t.Fatalf("view %+v", view)
	}
}

func TestGameAchievementsView(t *testing.T) {
	h := newHarness(t)
	h.writeProgress(t, `{"ACH_B":{"earned":true,"earned_time":1700000000},"OLD_KEY":{"earned":true}}`)
	view, err := h.svc.GameAchievements(context.Background(), "g1")
	if err != nil {
		t.Fatalf("view: %v", err)
	}
	if view.Total != 4 || view.Unlocked != 2 || view.Source != SourceLocal || view.SchemaError != "" {
		t.Fatalf("view %+v", view)
	}
	wantOrder := []string{"ACH_A", "ACH_B", "ACH_C", "OLD_KEY"}
	for i, key := range wantOrder {
		if view.Items[i].Key != key {
			t.Fatalf("order %v", view.Items)
		}
	}
	items := keysOf(view)
	if items["ACH_A"].Unlocked || !items["ACH_B"].Unlocked || !items["ACH_B"].Hidden || items["ACH_B"].Name != "Beta" {
		t.Fatalf("items %+v", items)
	}
	if items["OLD_KEY"].Name != "" || !items["OLD_KEY"].Unlocked || items["OLD_KEY"].UnlockedAt != "" {
		t.Fatalf("unlocked key missing from the schema: %+v", items["OLD_KEY"])
	}
}

func TestGameAchievementsFindsAFileThatIsNotThereYet(t *testing.T) {
	h := newHarness(t)
	view, err := h.svc.GameAchievements(context.Background(), "g1")
	if err != nil {
		t.Fatalf("view: %v", err)
	}
	if view.Source != SourceNone || view.Total != 3 || view.Unlocked != 0 {
		t.Fatalf("view %+v", view)
	}
}

func TestSchemaCache(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	get := func() {
		t.Helper()
		if _, err := h.svc.GameAchievements(ctx, "g1"); err != nil {
			t.Fatalf("view: %v", err)
		}
	}
	get()
	get()
	if n := h.schema.count(); n != 1 {
		t.Fatalf("fresh cache must be reused: %d fetches", n)
	}
	h.clock.advance(schemaTTL - time.Minute)
	get()
	if n := h.schema.count(); n != 1 {
		t.Fatalf("cache expired early: %d fetches", n)
	}
	h.clock.advance(2 * time.Minute)
	get()
	if n := h.schema.count(); n != 2 {
		t.Fatalf("cache must refresh after the TTL: %d fetches", n)
	}
	h.lang = "en"
	get()
	if n := h.schema.count(); n != 3 || h.schema.langs[2] != "en" {
		t.Fatalf("a language change must refetch: %d fetches, langs %v", n, h.schema.langs)
	}

	h.clock.advance(2 * schemaTTL)
	h.schema.set(nil, fmt.Errorf("%w: HTTP 503", ErrSchemaDown))
	view, err := h.svc.GameAchievements(ctx, "g1")
	if err != nil || view.SchemaError != SchemaUnavailable || view.Total != 3 || view.Items[0].Name != "Alpha" {
		t.Fatalf("a stale list must outlive a failed refresh: view %+v err %v", view, err)
	}
}

func TestNegativeSchemaAnswerIsCached(t *testing.T) {
	h := newHarness(t)
	h.schema.set(nil, ErrNoSchema)
	for range 3 {
		if _, err := h.svc.GameAchievements(context.Background(), "g1"); err != nil {
			t.Fatalf("view: %v", err)
		}
	}
	if n := h.schema.count(); n != 1 {
		t.Fatalf("%d fetches for a game known to have no achievements", n)
	}
}

func TestRescanPicksUpNewProgress(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	if _, err := h.svc.GameAchievements(ctx, "g1"); err != nil {
		t.Fatal(err)
	}
	h.writeProgress(t, `{"ACH_C":{"earned":true}}`)
	view, err := h.svc.Rescan(ctx, "g1")
	if err != nil || view.Unlocked != 1 || !keysOf(view)["ACH_C"].Unlocked {
		t.Fatalf("view %+v err %v", view, err)
	}
}

func TestPersistFailureRollsMemoryBack(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.writeProgress(t, `{"ACH_A":{"earned":true}}`)
	failing := errors.New("disk full")
	h.svc.store.write = func(string, int, any) error { return failing }

	if _, err := h.svc.Scan(ctx, "g1"); !errors.Is(err, failing) {
		t.Fatalf("scan err = %v", err)
	}
	if len(h.svc.store.games) != 0 {
		t.Fatalf("memory kept what the disk never got: %+v", h.svc.store.games)
	}
	if got := h.events.snapshot(); len(got) != 0 {
		t.Fatalf("announced an unlock that was not saved: %+v", got)
	}

	h.svc.store.write = func(path string, version int, data any) error { return nil }
	prog, err := h.svc.Scan(ctx, "g1")
	if err != nil || !slices.Equal(prog.NewKeys, []string{"ACH_A"}) {
		t.Fatalf("after the disk recovered: %+v %v", prog, err)
	}
}

func TestSessionStoppedScansInTheBackground(t *testing.T) {
	h := newHarness(t)
	done := make(chan Event, 1)
	h.svc.emit = func(ev Event) { done <- ev }
	h.writeProgress(t, `{"ACH_A":{"earned":true}}`)

	h.svc.SessionStopped("g1")
	h.svc.mu.Lock()
	idle := h.svc.ctx == nil
	h.svc.mu.Unlock()
	if !idle {
		t.Fatal("test setup: service must not be running yet")
	}
	select {
	case ev := <-done:
		t.Fatalf("a stopped service scanned: %+v", ev)
	default:
	}

	if err := h.svc.ServiceStartup(context.Background(), application.ServiceOptions{}); err != nil {
		t.Fatalf("startup: %v", err)
	}
	if err := h.svc.ServiceStartup(context.Background(), application.ServiceOptions{}); err == nil {
		t.Fatal("second startup must fail")
	}
	h.svc.SessionStopped("g1")
	h.svc.wg.Wait()
	select {
	case ev := <-done:
		t.Fatalf("the first session of a game is a baseline, got %+v", ev)
	default:
	}
	if rec, ok := h.svc.record("g1"); !ok || len(rec.Unlocked) != 1 {
		t.Fatalf("the baseline record was not created: %+v %v", rec, ok)
	}

	h.writeProgress(t, `{"ACH_A":{"earned":true},"ACH_B":{"earned":true}}`)
	h.svc.SessionStopped("g1")
	ev := <-done
	if ev.GameID != "g1" || !slices.Equal(ev.Keys, []string{"ACH_B"}) {
		t.Fatalf("event %+v", ev)
	}
	if err := h.svc.ServiceShutdown(); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
	h.svc.SessionStopped("g1")
}

func TestSessionStoppedRacesWithViews(t *testing.T) {
	h := newHarness(t)
	h.writeProgress(t, `{"ACH_A":{"earned":true},"ACH_B":{"earned":true},"ACH_C":{"earned":true}}`)
	if err := h.svc.ServiceStartup(context.Background(), application.ServiceOptions{}); err != nil {
		t.Fatalf("startup: %v", err)
	}

	var wg sync.WaitGroup
	for range 8 {
		wg.Add(2)
		go func() {
			defer wg.Done()
			h.svc.SessionStopped("g1")
		}()
		go func() {
			defer wg.Done()
			if _, err := h.svc.GameAchievements(context.Background(), "g1"); err != nil {
				t.Errorf("view: %v", err)
			}
		}()
	}
	wg.Wait()
	view, err := h.svc.Rescan(context.Background(), "g1")
	if err != nil || view.Unlocked != 3 {
		t.Fatalf("view %+v err %v", view, err)
	}
	if err := h.svc.ServiceShutdown(); err != nil {
		t.Fatalf("shutdown: %v", err)
	}

	seen := map[string]int{}
	for _, ev := range h.events.snapshot() {
		for _, key := range ev.Keys {
			seen[key]++
		}
	}
	for key, n := range seen {
		if n != 1 {
			t.Fatalf("%s announced %d times: %+v", key, n, h.events.snapshot())
		}
	}
}

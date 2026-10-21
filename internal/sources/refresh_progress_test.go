package sources

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"sync"
	"testing"
	"time"

	"typhon/internal/catalog"
	"typhon/internal/matchprogress"
	"typhon/internal/sources/feed"
	"typhon/internal/titles"
)

type progressLog struct {
	mu     sync.Mutex
	events []refreshProgress
}

func (l *progressLog) emit(name string, data any) {
	if name != eventProgress {
		return
	}
	p, ok := data.(refreshProgress)
	if !ok {
		panic(fmt.Sprintf("progress payload is %T", data))
	}
	l.mu.Lock()
	l.events = append(l.events, p)
	l.mu.Unlock()
}

func (l *progressLog) stage(name string) []refreshProgress {
	l.mu.Lock()
	defer l.mu.Unlock()
	var out []refreshProgress
	for _, p := range l.events {
		if p.Stage == name {
			out = append(out, p)
		}
	}
	return out
}

func (l *progressLog) order() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	var out []string
	for _, p := range l.events {
		if len(out) == 0 || out[len(out)-1] != p.Stage {
			out = append(out, p.Stage)
		}
	}
	return out
}

func dones(events []refreshProgress) []int {
	out := make([]int, len(events))
	for i, p := range events {
		out[i] = p.Done
	}
	return out
}

type batchRemote struct{ batch int }

func (r *batchRemote) Browse(context.Context, catalog.GameQuery) (catalog.GamePage, error) {
	return catalog.GamePage{}, nil
}

func (r *batchRemote) MatchReleases(ctx context.Context, qs []catalog.ReleaseQuery) ([]catalog.ReleaseMatch, error) {
	for done := r.batch; ; done += r.batch {
		done = min(done, len(qs))
		matchprogress.Report(ctx, done, len(qs))
		if done == len(qs) {
			break
		}
	}
	return make([]catalog.ReleaseMatch, len(qs)), nil
}

func manyEntries(t *testing.T, n int, hinted bool) string {
	t.Helper()
	downloads := make([]map[string]any, n)
	for i := range downloads {
		d := map[string]any{
			"title": fmt.Sprintf("Game Number %d v1.0", i),
			"uris":  []string{magnetOf(fmt.Sprintf("%08x", i+1))},
		}
		if hinted && i%2 == 0 {
			d["game"] = fmt.Sprintf("Game Number %d", i)
		}
		downloads[i] = d
	}
	data, err := json.Marshal(map[string]any{"name": "Big", "version": 1, "downloads": downloads})
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestRefreshReportsEveryStageWithGrowingProgress(t *testing.T) {
	s, cat, _ := testService(t)
	events := &progressLog{}
	s.events = events.emit
	cat.SetRemoteCatalog(&batchRemote{batch: 100})
	server := newFeedServer(t, manyEntries(t, 1200, false))
	src := addSource(t, s, server.url())

	if got, want := events.order(), []string{stageFetch, stageParse, stageMatch, stageApply, stageSave}; !slices.Equal(got, want) {
		t.Fatalf("stage order = %v, want %v", got, want)
	}
	events.mu.Lock()
	all := slices.Clone(events.events)
	events.mu.Unlock()
	for _, p := range all {
		if p.SourceID != src.ID {
			t.Fatalf("event %+v is for another source", p)
		}
	}
	for _, name := range []string{stageFetch, stageParse, stageMatch, stageApply, stageSave} {
		list := events.stage(name)
		if len(list) < 2 {
			t.Fatalf("%s sent %d events, want a start and an end", name, len(list))
		}
		for i, p := range list {
			if p.Total != list[0].Total {
				t.Fatalf("%s total changed: %+v", name, list)
			}
			if p.Done < 0 || p.Done > p.Total || (i > 0 && p.Done < list[i-1].Done) {
				t.Fatalf("%s done is not monotonic: %v of %d", name, dones(list), p.Total)
			}
		}
		if last := list[len(list)-1]; last.Done != last.Total {
			t.Fatalf("%s ends at %d of %d", name, last.Done, last.Total)
		}
	}
	if got, want := dones(events.stage(stageParse)), []int{500, 1000, 1200}; !slices.Equal(got, want) {
		t.Fatalf("parse progress = %v, want one event per %d entries and the end", got, progressStep)
	}
	match := events.stage(stageMatch)
	if got, want := dones(match), []int{0, 100, 200, 300, 400, 500, 600, 700, 800, 900, 1000, 1100, 1200}; !slices.Equal(got, want) || match[0].Total != 1200 {
		t.Fatalf("match progress = %v of %d, want one event per batch of 100", dones(match), match[0].Total)
	}
}

func TestRefreshClosesMatchStageWhenTheMatcherReportsNoBatches(t *testing.T) {
	s, cat, _ := testService(t)
	events := &progressLog{}
	s.events = events.emit
	remote := &cachingTestRemote{games: map[string]catalog.Game{}}
	cat.SetRemoteCatalog(remote)
	server := newFeedServer(t, manyEntries(t, 3, false))
	addSource(t, s, server.url())

	if got, want := dones(events.stage(stageMatch)), []int{0, 3}; !slices.Equal(got, want) {
		t.Fatalf("match progress = %v, want %v", got, want)
	}
}

func TestRefreshOfUnchangedFeedStillReportsEveryStage(t *testing.T) {
	s, cat, _ := testService(t)
	cat.SetRemoteCatalog(&batchRemote{batch: 100})
	server := newFeedServer(t, manyEntries(t, 5, false))
	src := addSource(t, s, server.url())
	events := &progressLog{}
	s.events = events.emit

	if _, err := s.RefreshSource(src.ID); err != nil {
		t.Fatal(err)
	}
	if got, want := events.order(), []string{stageFetch, stageParse, stageMatch, stageApply, stageSave}; !slices.Equal(got, want) {
		t.Fatalf("stage order = %v, want %v", got, want)
	}
}

type recordHandler struct {
	mu      *sync.Mutex
	records *[]slog.Record
}

func (h recordHandler) Enabled(context.Context, slog.Level) bool { return true }
func (h recordHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	*h.records = append(*h.records, r.Clone())
	h.mu.Unlock()
	return nil
}
func (h recordHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h recordHandler) WithGroup(string) slog.Handler      { return h }

func TestRefreshLogsOneLineWithEveryStageDuration(t *testing.T) {
	var (
		mu      sync.Mutex
		records []slog.Record
	)
	previous := slog.Default()
	slog.SetDefault(slog.New(recordHandler{mu: &mu, records: &records}))
	t.Cleanup(func() { slog.SetDefault(previous) })

	s, cat, _ := testService(t)
	cat.SetRemoteCatalog(&batchRemote{batch: 100})
	server := newFeedServer(t, manyEntries(t, 5, false))
	src := addSource(t, s, server.url())

	mu.Lock()
	defer mu.Unlock()
	var lines []map[string]int64
	for _, r := range records {
		if r.Message != "source refresh stages" {
			continue
		}
		attrs := map[string]int64{}
		r.Attrs(func(a slog.Attr) bool {
			if a.Key == "source_id" {
				if a.Value.String() != src.ID {
					t.Errorf("source_id = %s, want %s", a.Value.String(), src.ID)
				}
				return true
			}
			attrs[a.Key] = a.Value.Int64()
			return true
		})
		lines = append(lines, attrs)
	}
	if len(lines) != 1 {
		t.Fatalf("got %d stage lines, want exactly one per refresh", len(lines))
	}
	for _, key := range []string{"fetch_ms", "parse_ms", "match_ms", "apply_ms", "save_ms"} {
		if _, ok := lines[0][key]; !ok {
			t.Fatalf("stage line %v misses %s", lines[0], key)
		}
	}
}

type titleCalls struct {
	mu    sync.Mutex
	parse int
	names int
}

func (c *titleCalls) memo() *titleMemo {
	m := newTitleMemo()
	parse, names := m.parse, m.matchNames
	m.parse = func(raw string) titles.Parsed {
		c.mu.Lock()
		c.parse++
		c.mu.Unlock()
		return parse(raw)
	}
	m.matchNames = func(raw string) []string {
		c.mu.Lock()
		c.names++
		c.mu.Unlock()
		return names(raw)
	}
	return m
}

func (c *titleCalls) take() (parse, names int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	parse, names = c.parse, c.names
	c.parse, c.names = 0, 0
	return parse, names
}

func TestRefreshParsesEachReleaseTitleAtMostTwice(t *testing.T) {
	const n = 40
	s, cat, _ := testService(t)
	calls := &titleCalls{}
	s.newMemo = calls.memo
	cat.SetRemoteCatalog(&cachingTestRemote{games: map[string]catalog.Game{
		"Game Number 0": {ID: "server-0", Title: "Game Number 0", ExternalIDs: catalog.ExternalIDs{IGDB: "1"}},
	}})
	server := newFeedServer(t, manyEntries(t, n, true))
	src := addSource(t, s, server.url())

	check := func(step string) {
		t.Helper()
		parse, names := calls.take()
		if parse == 0 || names == 0 {
			t.Fatalf("%s: parse=%d names=%d, the counter saw nothing", step, parse, names)
		}
		if parse > 2*n || names > 2*n {
			t.Fatalf("%s: %d Parse and %d MatchNames calls for %d releases, want at most %d of each", step, parse, names, n, 2*n)
		}
	}
	check("first import")

	changed := manyEntries(t, n, true)
	server.set(changed, `"v2"`)
	if _, err := s.RefreshSource(src.ID); err != nil {
		t.Fatal(err)
	}
	check("changed etag, same body")

	if _, err := s.RefreshSource(src.ID); err != nil {
		t.Fatal(err)
	}
	check("feed not modified")
}

func TestParseFeedEntriesStopsWhenContextIsCancelled(t *testing.T) {
	entries := make([]feed.Entry, 1000)
	for i := range entries {
		entries[i] = feed.Entry{Title: fmt.Sprintf("Game %d v1.0", i), URIs: []string{magnetOf(fmt.Sprintf("%08x", i+1))}}
	}
	t.Run("cancelled before the call", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		list, err := parseFeedEntries(ctx, "src", entries, time.Now(), newTitleMemo(), nil)
		if !errors.Is(err, context.Canceled) || list != nil {
			t.Fatalf("list=%d err=%v, want nothing and context.Canceled", len(list), err)
		}
	})
	t.Run("cancelled between records", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		var seen []int
		list, err := parseFeedEntries(ctx, "src", entries, time.Now(), newTitleMemo(), func(done, total int) {
			seen = append(seen, done)
			cancel()
		})
		if !errors.Is(err, context.Canceled) || list != nil {
			t.Fatalf("list=%d err=%v, want nothing and context.Canceled", len(list), err)
		}
		if !slices.Equal(seen, []int{progressStep}) {
			t.Fatalf("progress after cancel = %v, want it to stop right away", seen)
		}
	})
}

func TestMergeReleasesStopsWhenContextIsCancelled(t *testing.T) {
	now := time.Now()
	existing := parseEntries("src", []feed.Entry{{Title: "Old Game v1.0", URIs: []string{magnetOf("11")}}}, now)
	incoming := parseEntries("src", []feed.Entry{{Title: "New Game v1.0", URIs: []string{magnetOf("22")}}}, now)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	merged, summary, err := mergeReleases(ctx, existing, incoming, now, false)
	if !errors.Is(err, context.Canceled) || merged != nil || summary != (Summary{}) {
		t.Fatalf("merged=%v summary=%+v err=%v, want nothing and context.Canceled", merged, summary, err)
	}
}

func TestReparseListAndResolveStopWhenContextIsCancelled(t *testing.T) {
	s, cat, _ := testService(t)
	cat.SetRemoteCatalog(&cachingTestRemote{})
	list := parseEntries("src", []feed.Entry{{Title: "Game v1.0", URIs: []string{magnetOf("11")}}}, time.Now())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := reparseList(ctx, newTitleMemo(), list, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("reparseList err = %v, want context.Canceled", err)
	}
	resolved, partial, err := s.resolveRemoteReleases(ctx, list, s.newRun("src"))
	if !errors.Is(err, context.Canceled) || resolved != nil || partial != nil {
		t.Fatalf("resolveRemoteReleases = %v %v %v, want nothing and context.Canceled", resolved, partial, err)
	}
}

func TestSettleStopsWhenContextIsCancelledAndKeepsTheStoredReleases(t *testing.T) {
	s, _, _ := testService(t)
	server := newFeedServer(t, feedBody(t, "Source", feedEntry{DistributionID: "a", Title: "Alpha Game v1.0", URIs: []string{magnetOf("aa")}}))
	src := addSource(t, s, server.url())
	path := s.store.releasesPath(src.ID)
	ageFile(t, path)
	s.mu.Lock()
	stored := s.releases[src.ID]
	s.mu.Unlock()

	incoming := parseEntries(src.ID, []feed.Entry{{Title: "Beta Game v1.0", URIs: []string{magnetOf("bb")}}}, time.Now())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	summary, err := s.settle(ctx, src.ID, incoming, feed.Result{}, time.Now(), false, false, nil, nil, s.newRun(src.ID))
	if !errors.Is(err, context.Canceled) || summary.Error == "" {
		t.Fatalf("summary=%+v err=%v, want context.Canceled", summary, err)
	}
	s.mu.Lock()
	same := len(s.releases[src.ID]) == len(stored) && &s.releases[src.ID][0] == &stored[0]
	s.mu.Unlock()
	if !same {
		t.Fatal("a cancelled settle replaced the stored releases")
	}
	if !modTime(t, path).Equal(longAgo) {
		t.Fatal("a cancelled settle wrote the releases file")
	}
}

func TestRefreshCancelledWhileParsingStoresNothing(t *testing.T) {
	s, _, _ := testService(t)
	server := newFeedServer(t, manyEntries(t, 1200, false))
	s.events = func(name string, data any) {
		if p, ok := data.(refreshProgress); ok && p.Stage == stageParse && p.Done == progressStep {
			s.cancel()
		}
	}
	src, err := s.AddSource(server.url())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("AddSource err = %v, want context.Canceled", err)
	}
	s.mu.Lock()
	stored := len(s.releases[src.ID])
	s.mu.Unlock()
	if stored != 0 {
		t.Fatalf("%d releases stored by a cancelled refresh", stored)
	}
	if _, statErr := s.store.loadReleases(src.ID); statErr != nil {
		t.Fatal(statErr)
	}
}

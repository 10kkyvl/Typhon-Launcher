package watchlist

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"typhon/internal/sources"
	"typhon/internal/uierr"
)

type fakeReleases struct {
	mu     sync.Mutex
	byGame map[string][]sources.ReleaseGroup
}

func (f *fakeReleases) GetReleasesForGame(gameID string) []sources.ReleaseGroup {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]sources.ReleaseGroup(nil), f.byGame[gameID]...)
}

func (f *fakeReleases) set(gameID string, groups ...sources.ReleaseGroup) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.byGame == nil {
		f.byGame = map[string][]sources.ReleaseGroup{}
	}
	f.byGame[gameID] = groups
}

type fakeTitles map[string]string

func (f fakeTitles) TitleOf(id string) string { return f[id] }

func rel(id, version string, ignored bool) sources.ReleaseGroup {
	return sources.ReleaseGroup{Release: sources.Release{
		ID: id, SourceID: "src", Title: "Game " + id, Version: version, Ignored: ignored,
	}}
}

func newTestService(t *testing.T) (*Service, *fakeReleases, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "watchlist.json")
	rels := &fakeReleases{}
	s, err := NewServiceAt(path, rels, fakeTitles{"g1": "Game One", "g2": "Game Two"})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	return s, rels, path
}

func TestWatchSnapshotsKnownAndIsIdempotent(t *testing.T) {
	s, rels, _ := newTestService(t)
	rels.set("g1", rel("r1", "1.0", false), rel("r2", "1.1", false), rel("rx", "0.9", true))

	first, err := s.Watch("g1")
	if err != nil {
		t.Fatalf("watch: %v", err)
	}
	if first.Title != "Game One" || len(first.Known) != 2 {
		t.Fatalf("entry = %+v, want title and 2 known releases", first)
	}
	s.HandleSourcesRefreshed(context.Background())
	if got := s.Alerts(); len(got) != 0 {
		t.Fatalf("alerts = %+v, existing releases must not alert", got)
	}

	rels.set("g1")
	again, err := s.Watch("g1")
	if err != nil {
		t.Fatalf("second watch: %v", err)
	}
	if len(again.Known) != 2 || !again.AddedAt.Equal(first.AddedAt) {
		t.Fatalf("second watch = %+v, want the original entry", again)
	}
	if got := s.List(); len(got) != 1 {
		t.Fatalf("list = %+v, want one entry", got)
	}
	if !s.IsWatched("g1") || s.IsWatched("g2") {
		t.Fatal("IsWatched disagrees with the list")
	}
}

func TestWatchAndUnwatchErrors(t *testing.T) {
	s, _, _ := newTestService(t)
	cases := []struct {
		name string
		run  func() error
		code string
	}{
		{"watch empty id", func() error { _, err := s.Watch(""); return err }, "watchlist.empty_id"},
		{"watch blank id", func() error { _, err := s.Watch("  "); return err }, "watchlist.empty_id"},
		{"watch unknown game", func() error { _, err := s.Watch("nope"); return err }, "watchlist.game_not_found"},
		{"unwatch not watched", func() error { return s.Unwatch("g1") }, "watchlist.not_watched"},
		{"dismiss unknown alert", func() error { return s.DismissAlert("nope") }, "watchlist.alert_not_found"},
		{"dismiss alerts empty id", func() error { return s.DismissAlerts("") }, "watchlist.empty_id"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.run()
			if err == nil {
				t.Fatal("want an error")
			}
			if got := uierr.Code(err); got != tc.code {
				t.Fatalf("code = %q, want %q", got, tc.code)
			}
		})
	}
	if got := s.List(); len(got) != 0 {
		t.Fatalf("failed calls left entries: %+v", got)
	}
}

func TestHandleSourcesRefreshed(t *testing.T) {
	s, rels, _ := newTestService(t)
	rels.set("g1", rel("r1", "1.0", false))
	rels.set("g2", rel("q1", "1.0", false))
	if _, err := s.Watch("g1"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Watch("g2"); err != nil {
		t.Fatal(err)
	}

	rels.set("g1", rel("r1", "1.0", false), rel("r2", "1.1", false), rel("r3", "1.2", true))
	s.HandleSourcesRefreshed(context.Background())

	alerts := s.Alerts()
	if len(alerts) != 1 {
		t.Fatalf("alerts = %+v, want exactly the new visible release", alerts)
	}
	a := alerts[0]
	if a.GameID != "g1" || a.ReleaseID != "r2" || a.Version != "1.1" || a.SourceID != "src" || a.Title != "Game One" || a.ID == "" {
		t.Fatalf("alert = %+v", a)
	}
	for _, e := range s.List() {
		if e.GameID == "g1" && len(e.Known) != 2 {
			t.Fatalf("g1 known = %v, want r1 and r2", e.Known)
		}
		if e.GameID == "g2" && len(e.Known) != 1 {
			t.Fatalf("g2 known = %v, want untouched", e.Known)
		}
	}

	s.HandleSourcesRefreshed(context.Background())
	if got := s.Alerts(); len(got) != 1 {
		t.Fatalf("repeat call produced duplicates: %+v", got)
	}

	if err := s.DismissAlert(a.ID); err != nil {
		t.Fatalf("dismiss: %v", err)
	}
	if got := s.Alerts(); len(got) != 0 {
		t.Fatalf("alerts after dismiss = %+v", got)
	}
}

func TestHandleSourcesRefreshedHonoursCancelledContext(t *testing.T) {
	s, rels, _ := newTestService(t)
	rels.set("g1", rel("r1", "1.0", false))
	if _, err := s.Watch("g1"); err != nil {
		t.Fatal(err)
	}
	rels.set("g1", rel("r1", "1.0", false), rel("r2", "1.1", false))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	s.HandleSourcesRefreshed(ctx)
	if got := s.Alerts(); len(got) != 0 {
		t.Fatalf("cancelled refresh produced alerts: %+v", got)
	}
	if e := s.List()[0]; len(e.Known) != 1 {
		t.Fatalf("known = %v, cancelled refresh must not advance it", e.Known)
	}
}

func TestUnwatchDropsEntryAndItsAlerts(t *testing.T) {
	s, rels, path := newTestService(t)
	rels.set("g1", rel("r1", "1.0", false))
	rels.set("g2", rel("q1", "1.0", false))
	for _, id := range []string{"g1", "g2"} {
		if _, err := s.Watch(id); err != nil {
			t.Fatal(err)
		}
	}
	rels.set("g1", rel("r1", "1.0", false), rel("r2", "1.1", false))
	rels.set("g2", rel("q1", "1.0", false), rel("q2", "1.1", false))
	s.HandleSourcesRefreshed(context.Background())
	if len(s.Alerts()) != 2 {
		t.Fatalf("alerts = %+v, want 2", s.Alerts())
	}

	if err := s.Unwatch("g1"); err != nil {
		t.Fatalf("unwatch: %v", err)
	}
	if s.IsWatched("g1") {
		t.Fatal("g1 still watched")
	}
	if got := s.Alerts(); len(got) != 1 || got[0].GameID != "g2" {
		t.Fatalf("alerts = %+v, want only g2", got)
	}

	reloaded, err := NewServiceAt(path, rels, fakeTitles{"g2": "Game Two"})
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if got := reloaded.List(); len(got) != 1 || got[0].GameID != "g2" || len(reloaded.Alerts()) != 1 {
		t.Fatalf("reloaded = %+v / %+v", got, reloaded.Alerts())
	}
}

func TestDismissAlertsByGame(t *testing.T) {
	s, rels, _ := newTestService(t)
	rels.set("g1", rel("r1", "1.0", false))
	if _, err := s.Watch("g1"); err != nil {
		t.Fatal(err)
	}
	rels.set("g1", rel("r1", "1.0", false), rel("r2", "1.1", false), rel("r3", "1.2", false))
	s.HandleSourcesRefreshed(context.Background())
	if len(s.Alerts()) != 2 {
		t.Fatalf("alerts = %+v, want 2", s.Alerts())
	}
	if err := s.DismissAlerts("g1"); err != nil {
		t.Fatalf("dismiss: %v", err)
	}
	if len(s.Alerts()) != 0 {
		t.Fatalf("alerts = %+v, want none", s.Alerts())
	}
	if err := s.DismissAlerts("g1"); err != nil {
		t.Fatalf("dismiss with nothing left: %v", err)
	}
}

func TestPersistFailureRollsBackMemory(t *testing.T) {
	s, rels, _ := newTestService(t)
	rels.set("g1", rel("r1", "1.0", false))
	if _, err := s.Watch("g1"); err != nil {
		t.Fatal(err)
	}
	rels.set("g1", rel("r1", "1.0", false), rel("r2", "1.1", false))
	rels.set("g2", rel("q1", "1.0", false))

	file := filepath.Join(t.TempDir(), "plain-file")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	s.path = filepath.Join(file, "watchlist.json")

	cases := []struct {
		name string
		run  func() error
	}{
		{"watch", func() error { _, err := s.Watch("g2"); return err }},
		{"unwatch", func() error { return s.Unwatch("g1") }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.run()
			if err == nil {
				t.Fatal("want a persist error")
			}
			if uierr.Code(err) != "watchlist.save_failed" {
				t.Fatalf("code = %q", uierr.Code(err))
			}
			got := s.List()
			if len(got) != 1 || got[0].GameID != "g1" || len(got[0].Known) != 1 {
				t.Fatalf("memory not rolled back: %+v", got)
			}
			if !s.StatusOf().Degraded {
				t.Fatal("status must be degraded after a failed save")
			}
		})
	}

	s.HandleSourcesRefreshed(context.Background())
	if got := s.Alerts(); len(got) != 0 {
		t.Fatalf("failed save left alerts: %+v", got)
	}
	if e := s.List()[0]; len(e.Known) != 1 {
		t.Fatalf("known advanced despite failed save: %v", e.Known)
	}
	if !s.StatusOf().Degraded {
		t.Fatal("refresh failure must set degraded")
	}

	s.path = filepath.Join(t.TempDir(), "watchlist.json")
	s.HandleSourcesRefreshed(context.Background())
	if got := s.Alerts(); len(got) != 1 || got[0].ReleaseID != "r2" {
		t.Fatalf("release must be found again after recovery: %+v", got)
	}
	if s.StatusOf().Degraded {
		t.Fatal("status must clear after a successful save")
	}
}

func TestAlertsAreCapped(t *testing.T) {
	s, rels, _ := newTestService(t)
	rels.set("g1")
	if _, err := s.Watch("g1"); err != nil {
		t.Fatal(err)
	}
	var groups []sources.ReleaseGroup
	for i := 0; i < maxAlerts+10; i++ {
		groups = append(groups, rel(fmt.Sprintf("r%d", i), "1", false))
	}
	rels.set("g1", groups...)
	s.HandleSourcesRefreshed(context.Background())
	if got := len(s.Alerts()); got != maxAlerts {
		t.Fatalf("alerts = %d, want %d", got, maxAlerts)
	}
}

func TestConstructorRejectsNilPorts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "watchlist.json")
	if _, err := NewServiceAt(path, nil, fakeTitles{}); err == nil {
		t.Fatal("nil releases port must be an error")
	}
	if _, err := NewServiceAt(path, &fakeReleases{}, nil); err == nil {
		t.Fatal("nil titles port must be an error")
	}
}

func TestConcurrentWatchAndRefresh(t *testing.T) {
	s, rels, _ := newTestService(t)
	rels.set("g1", rel("r1", "1.0", false))
	rels.set("g2", rel("q1", "1.0", false))

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(3)
		go func() {
			defer wg.Done()
			if _, err := s.Watch("g1"); err != nil {
				t.Errorf("watch g1: %v", err)
			}
		}()
		go func() {
			defer wg.Done()
			if _, err := s.Watch("g2"); err != nil {
				t.Errorf("watch g2: %v", err)
			}
		}()
		go func(i int) {
			defer wg.Done()
			rels.set("g1", rel("r1", "1.0", false), rel(fmt.Sprintf("n%d", i), "2", false))
			s.HandleSourcesRefreshed(context.Background())
			_ = s.List()
			_ = s.Alerts()
		}(i)
	}
	wg.Wait()
	if got := s.List(); len(got) != 2 {
		t.Fatalf("list = %+v, want 2 entries", got)
	}
	if err := s.Unwatch("g1"); err != nil && !errors.Is(err, errNotWatched) {
		t.Fatalf("unwatch: %v", err)
	}
}

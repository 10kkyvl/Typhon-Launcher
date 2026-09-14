package catalog

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestBrowseSnapshotFreezesPreferencesBetweenPages(t *testing.T) {
	s, err := NewServiceAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	first, err := s.prepareBrowseSnapshot(GameQuery{Sort: "auto", HideNotInterested: true, Page: 1, PageSize: 60})
	if err != nil {
		t.Fatal(err)
	}
	if first.Sort != "popular" || first.Snapshot == "" {
		t.Fatalf("first query: %+v", first)
	}
	if err = s.SetNotInterested("00000000-0000-0000-0000-000000000001", true); err != nil {
		t.Fatal(err)
	}
	next, err := s.prepareBrowseSnapshot(GameQuery{Sort: "auto", HideNotInterested: true, Page: 2, PageSize: 60, Revision: 10, Snapshot: first.Snapshot})
	if err != nil {
		t.Fatal(err)
	}
	if next.ExcludeNotInterested != first.ExcludeNotInterested || next.Profile != first.Profile || next.Sort != first.Sort {
		t.Fatalf("personal snapshot changed: %+v => %+v", first, next)
	}
	fresh, err := s.prepareBrowseSnapshot(GameQuery{Sort: "auto", HideNotInterested: true, Page: 1})
	if err != nil {
		t.Fatal(err)
	}
	if fresh.ExcludeNotInterested == "" {
		t.Fatal("new first page did not pick up explicit dismissal")
	}
}

func TestRecommendationPreferencesRestartThroughServiceConstructor(t *testing.T) {
	dir := t.TempDir()
	s, err := NewServiceAt(dir)
	if err != nil {
		t.Fatal(err)
	}
	prefs := s.GetRecommendationPreferences()
	if !prefs.HideNotInterested {
		t.Fatal("first profile must hide dismissed games")
	}
	prefs.DefaultSort = "rating"
	prefs.Genre = "Action"
	prefs.HideLibrary = true
	if err = s.SaveRecommendationPreferences(prefs); err != nil {
		t.Fatal(err)
	}
	if err = s.SetNotInterested("00000000-0000-0000-0000-000000000001", true); err != nil {
		t.Fatal(err)
	}
	restarted, err := NewServiceAt(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(s.GetRecommendationPreferences(), restarted.GetRecommendationPreferences()) {
		t.Fatal("constructor did not restore preferences")
	}
	if err = restarted.SetNotInterested("00000000-0000-0000-0000-000000000001", false); err != nil {
		t.Fatal(err)
	}
	final, err := NewServiceAt(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(final.GetRecommendationPreferences().NotInterested) != 0 {
		t.Fatal("undo did not survive restart")
	}
}

func TestExpiredPersonalSnapshotRequiresExplicitRestart(t *testing.T) {
	s, err := NewServiceAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	q, err := s.prepareBrowseSnapshot(GameQuery{Page: 1})
	if err != nil {
		t.Fatal(err)
	}
	s.browseSnapshots[q.Snapshot] = browseSnapshot{query: q, used: time.Now().Add(-time.Hour)}
	if _, err = s.prepareBrowseSnapshot(GameQuery{Page: 2, Snapshot: q.Snapshot}); err == nil {
		t.Fatal("expired snapshot accepted")
	}
}

func TestAutoBrowseBuildsRecommendationProfileFromOneLibrarySnapshot(t *testing.T) {
	s, err := NewServiceAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"known-a", "known-b", "known-c"} {
		if _, err = s.AddGame(Game{ID: id, Title: "Known"}); err != nil {
			t.Fatal(err)
		}
	}
	calls := 0
	s.SetRecommendationLibrarySource(func() []RecommendationLibraryItem {
		calls++
		return []RecommendationLibraryItem{
			{CanonicalGameID: "known-a", Favorite: true},
			{CanonicalGameID: "known-b", Favorite: true},
			{CanonicalGameID: "known-c", Favorite: true},
		}
	})
	s.SetRemoteCatalog(&remoteFixture{page: GamePage{Items: []Game{{ID: "remote", Title: "Remote"}}}})
	if _, err = s.BrowseGames(GameQuery{Sort: "auto", Page: 1}); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("recommendation source called %d times, want 1", calls)
	}
}

func TestPersonalizedContinuationWithoutSnapshotRequiresRestart(t *testing.T) {
	s, err := NewServiceAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.prepareBrowseSnapshot(GameQuery{Sort: "for-you", Page: 2}); !errors.Is(err, ErrCatalogChanged) {
		t.Fatalf("personalized continuation error = %v, want ErrCatalogChanged", err)
	}
}

func TestPopularContinuationWithoutSnapshotRemainsAvailable(t *testing.T) {
	s, err := NewServiceAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	remote := &remoteFixture{page: GamePage{Items: []Game{{ID: "second", Title: "Second"}}, Page: 2, PageSize: 1}}
	s.SetRemoteCatalog(remote)
	page, err := s.BrowseGames(GameQuery{Sort: "popular", Page: 2, PageSize: 1})
	if err != nil || len(page.Items) != 1 || remote.calls != 1 {
		t.Fatalf("popular continuation = %+v, err=%v, calls=%d", page, err, remote.calls)
	}
}

type fallbackSnapshotRemote struct{ queries []GameQuery }

func (r *fallbackSnapshotRemote) Browse(_ context.Context, q GameQuery) (GamePage, error) {
	r.queries = append(r.queries, q)
	if q.Sort == "for-you" {
		return GamePage{}, errors.New("personalization unavailable")
	}
	id := "first"
	if q.Page > 1 {
		id = "second"
	}
	return GamePage{Items: []Game{{ID: id, Title: id, SortTitle: id}}, Page: q.Page, PageSize: 1, Total: 2, Revision: 1}, nil
}
func TestPersonalizationFailurePinsGeneralFallbackForNextPage(t *testing.T) {
	s, err := NewServiceAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	remote := &fallbackSnapshotRemote{}
	s.SetRemoteCatalog(remote)
	first, err := s.BrowseGames(GameQuery{Sort: "for-you", Page: 1, PageSize: 1})
	if err != nil {
		t.Fatal(err)
	}
	if !first.PersonalizationFallback || first.Snapshot == "" || len(first.Items) != 1 {
		t.Fatalf("fallback: %+v", first)
	}
	next, err := s.BrowseGames(GameQuery{Sort: "for-you", Page: 2, PageSize: 1, Snapshot: first.Snapshot, Revision: first.Revision})
	if err != nil {
		t.Fatal(err)
	}
	if len(next.Items) != 1 || next.Items[0].ID == first.Items[0].ID {
		t.Fatalf("bad continuation: %+v", next)
	}
	if len(remote.queries) != 3 || remote.queries[2].Sort != "popular" {
		t.Fatalf("fallback query was not pinned: %+v", remote.queries)
	}
}

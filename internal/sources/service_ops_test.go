package sources

import (
	"errors"
	"strings"
	"testing"

	"typhon/internal/catalog"
)

func TestSourceCallsReportAnUnknownSource(t *testing.T) {
	s, _, _ := testService(t)

	if _, err := s.GetSource("nope"); !errors.Is(err, errSourceNotFound) {
		t.Errorf("GetSource error = %v", err)
	}
	if _, err := s.GetSourceDetails("nope"); !errors.Is(err, errSourceNotFound) {
		t.Errorf("GetSourceDetails error = %v", err)
	}
	if err := s.RemoveSource("nope"); !errors.Is(err, errSourceNotFound) {
		t.Errorf("RemoveSource error = %v", err)
	}
	if err := s.SetSourceEnabled("nope", true); !errors.Is(err, errSourceNotFound) {
		t.Errorf("SetSourceEnabled error = %v", err)
	}
	if _, err := s.RefreshSource("nope"); !errors.Is(err, errSourceNotFound) {
		t.Errorf("RefreshSource error = %v", err)
	}
	if err := s.IgnoreRelease("nope", true); !errors.Is(err, errReleaseNotFound) {
		t.Errorf("IgnoreRelease error = %v", err)
	}
	if _, err := s.GetRelease("nope"); !errors.Is(err, errReleaseNotFound) {
		t.Errorf("GetRelease error = %v", err)
	}
	if _, err := s.PrepareDownload("nope"); !errors.Is(err, errReleaseNotFound) {
		t.Errorf("PrepareDownload error = %v", err)
	}
	if _, ok := s.FindRelease("nope"); ok {
		t.Error("FindRelease found a release that does not exist")
	}
}

func TestAddSourceStoresTheNormalizedURLAndRecognisesItAgain(t *testing.T) {
	s, _, _ := testService(t)
	server := newFeedServer(t, feedBody(t, "Feed", feedEntry{Title: "Game A", URIs: []string{magnetOf("a")}}))

	messy := "  " + strings.ToUpper(server.url()[:len("http")]) + server.url()[len("http"):] + "  "
	src := addSource(t, s, messy)
	if src.URL != server.url() {
		t.Fatalf("stored url = %q, want %q", src.URL, server.url())
	}
	if _, err := s.AddSource(server.url()); !errors.Is(err, errSourceExists) {
		t.Fatalf("the same feed typed cleanly was added twice: %v", err)
	}
	if got := len(s.ListSources()); got != 1 {
		t.Fatalf("sources = %d, want 1", got)
	}
}

func TestGetSourceDetailsCountsEveryBucket(t *testing.T) {
	s, _, _ := testService(t)
	feedA := feedEntry{Title: "Game A", URIs: []string{magnetOf("a")}}
	feedB := feedEntry{Title: "Game B", URIs: []string{magnetOf("b")}}
	feedC := feedEntry{Title: "Game C", URIs: []string{magnetOf("c")}}
	feedD := feedEntry{Title: "Game D", URIs: []string{magnetOf("d")}}
	server := newFeedServer(t, feedBody(t, "Feed", feedA, feedB, feedC))
	src := addSource(t, s, server.url())

	var ignoreID string
	for _, item := range releasesOf(t, s, src.ID, "all") {
		if item.Release.RawTitle == "Game B" {
			ignoreID = item.Release.ID
		}
	}
	if ignoreID == "" {
		t.Fatal("release B not found")
	}
	if err := s.IgnoreRelease(ignoreID, true); err != nil {
		t.Fatal(err)
	}

	server.set(feedBody(t, "Feed", feedA, feedB, feedD), `"v2"`)
	if _, err := s.RefreshSource(src.ID); err != nil {
		t.Fatalf("refresh: %v", err)
	}

	details, err := s.GetSourceDetails(src.ID)
	if err != nil {
		t.Fatal(err)
	}
	if details.Total != 4 || details.Ignored != 1 || details.Removed != 1 || details.Available != 2 {
		t.Fatalf("details = %+v, want total 4: 1 ignored, 1 removed, 2 available", details)
	}
	if details.Available+details.Ignored+details.Removed != details.Total {
		t.Fatalf("the buckets do not add up to the total: %+v", details)
	}
	if details.New != 1 {
		t.Fatalf("new = %d, want the one release that appeared after the first import", details.New)
	}

	if err := s.AcknowledgeNew(src.ID); err != nil {
		t.Fatal(err)
	}
	after, err := s.GetSourceDetails(src.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.New != 0 || after.Total != details.Total {
		t.Fatalf("details after acknowledging = %+v", after)
	}
}

func TestRefreshAllSkipsDisabledAndFailingSources(t *testing.T) {
	s, _, _ := testService(t)
	healthy := newFeedServer(t, feedBody(t, "Healthy", feedEntry{Title: "Game A", URIs: []string{magnetOf("a")}}))
	off := newFeedServer(t, feedBody(t, "Off", feedEntry{Title: "Game B", URIs: []string{magnetOf("b")}}))
	broken := newFeedServer(t, feedBody(t, "Broken", feedEntry{Title: "Game C", URIs: []string{magnetOf("c")}}))

	healthySrc := addSource(t, s, healthy.url())
	offSrc := addSource(t, s, off.url())
	brokenSrc := addSource(t, s, broken.url())
	if err := s.SetSourceEnabled(offSrc.ID, false); err != nil {
		t.Fatal(err)
	}
	broken.fail(500)
	offBefore, healthyBefore := off.count(), healthy.count()

	summaries := s.RefreshAll()

	if len(summaries) != 1 || summaries[0].SourceID != healthySrc.ID {
		t.Fatalf("summaries = %+v, want only the healthy source", summaries)
	}
	if off.count() != offBefore {
		t.Fatal("a disabled source was fetched")
	}
	if healthy.count() != healthyBefore+1 {
		t.Fatalf("healthy source fetched %d times, want once", healthy.count()-healthyBefore)
	}
	failed, err := s.GetSource(brokenSrc.ID)
	if err != nil {
		t.Fatal(err)
	}
	if failed.Health != HealthError || failed.LastError == "" {
		t.Fatalf("broken source = %+v, want its failure recorded", failed)
	}
	if again, err := s.GetSource(healthySrc.ID); err != nil || again.Health != HealthHealthy {
		t.Fatalf("a neighbour's failure touched the healthy source: %+v, %v", again, err)
	}
}

func TestSetSourceEnabledTogglesAndKeepsTheReleases(t *testing.T) {
	s, _, _ := testService(t)
	server := newFeedServer(t, feedBody(t, "Feed", feedEntry{Title: "Game A", URIs: []string{magnetOf("a")}}))
	src := addSource(t, s, server.url())

	if err := s.SetSourceEnabled(src.ID, false); err != nil {
		t.Fatal(err)
	}
	off, err := s.GetSource(src.ID)
	if err != nil {
		t.Fatal(err)
	}
	if off.Enabled || off.Status != StatusDisabled {
		t.Fatalf("source = %+v, want disabled", off)
	}
	if _, err := s.refresh(s.ctx, src.ID, true); !errors.Is(err, errSourceDisabled) {
		t.Fatalf("scheduled refresh of a disabled source: %v", err)
	}
	if got := len(releasesOf(t, s, src.ID, "all")); got != 1 {
		t.Fatalf("disabling dropped the releases: %d left", got)
	}

	if err := s.SetSourceEnabled(src.ID, true); err != nil {
		t.Fatal(err)
	}
	on, err := s.GetSource(src.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !on.Enabled || on.Status != StatusActive {
		t.Fatalf("source = %+v, want active again", on)
	}
}

func TestReleaseLookupsFollowTheCatalogAndHideIgnored(t *testing.T) {
	s, cat, _ := testService(t)
	game, err := cat.AddGame(catalog.Game{Title: "Cyberpunk 2077"})
	if err != nil {
		t.Fatal(err)
	}
	server := newFeedServer(t, feedBody(t, "Feed", feedEntry{Title: "Cyberpunk.2077.Ultimate.Edition.v2.31", URIs: []string{magnetOf("aa")}}))
	src := addSource(t, s, server.url())

	items := releasesOf(t, s, src.ID, "all")
	if len(items) != 1 || items[0].Release.CanonicalGameID == nil {
		t.Fatalf("release did not match the catalog game: %+v", items)
	}
	releaseID := items[0].Release.ID

	if groups := s.GetReleasesForTitle("Cyberpunk 2077"); len(groups) != 1 || groups[0].Release.ID != releaseID {
		t.Fatalf("GetReleasesForTitle = %+v", groups)
	}
	if groups := s.GetReleasesForTitle("A Game Nobody Added"); len(groups) != 0 {
		t.Fatalf("an unknown title returned %+v", groups)
	}
	byTitle := s.ReleasesFor("", "Cyberpunk 2077")
	byID := s.ReleasesFor(game.ID, "")
	if len(byTitle) != 1 || len(byID) != 1 || byTitle[0].ID != releaseID || byID[0].ID != releaseID {
		t.Fatalf("ReleasesFor by title %+v, by id %+v", byTitle, byID)
	}
	if got := s.ReleasesFor("", ""); len(got) != 0 {
		t.Fatalf("ReleasesFor with no identity returned %+v", got)
	}
	found, ok := s.FindRelease(releaseID)
	if !ok || found.ID != releaseID {
		t.Fatalf("FindRelease = %+v, %v", found, ok)
	}

	if err := s.IgnoreRelease(releaseID, true); err != nil {
		t.Fatal(err)
	}
	if got := s.ReleasesFor(game.ID, ""); len(got) != 0 {
		t.Fatalf("an ignored release is still offered: %+v", got)
	}
	if got := s.GetReleasesForTitle("Cyberpunk 2077"); len(got) != 0 {
		t.Fatalf("an ignored release is still grouped: %+v", got)
	}
	if _, ok := s.FindRelease(releaseID); !ok {
		t.Fatal("FindRelease must still see an ignored release")
	}
}

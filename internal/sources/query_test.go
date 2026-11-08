package sources

import (
	"reflect"
	"testing"
	"time"

	"typhon/internal/catalog"
)

func queryFixture(t *testing.T) *Service {
	t.Helper()
	day := func(n int) *time.Time {
		at := time.Date(2026, 1, n, 0, 0, 0, 0, time.UTC)
		return &at
	}
	matched := availableRelease("r-matched", "src", "Beta", "beta", "g1", "1.0")
	matched.Size, matched.UploadedAt = 300, day(3)
	review := availableRelease("r-review", "src", "Alpha", "alpha", "", "1.0")
	review.MatchStatus, review.Size, review.UploadedAt = catalog.StatusReview, 100, day(5)
	unmatched := availableRelease("r-unmatched", "src", "Gamma", "gamma", "", "1.0")
	unmatched.Size, unmatched.UploadedAt = 200, day(1)
	removed := availableRelease("r-removed", "src", "Delta", "delta", "g2", "1.0")
	removed.Availability, removed.Size = AvailabilityRemoved, 50
	fresh := availableRelease("r-new", "src", "Epsilon", "epsilon", "", "1.0")
	fresh.New, fresh.Size = true, 400
	ignored := availableRelease("r-ignored", "src", "Zeta", "zeta", "g3", "1.0")
	ignored.Ignored, ignored.Size, ignored.UploadedAt = true, 500, day(9)

	return searchService(t,
		[]*Source{{ID: "src", Name: "Feed", Enabled: true}},
		map[string][]*Release{"src": {matched, review, unmatched, removed, fresh, ignored}},
	)
}

func idsOf(page ReleasePage) []string {
	ids := make([]string, 0, len(page.Items))
	for _, item := range page.Items {
		ids = append(ids, item.Release.ID)
	}
	return ids
}

func TestQueryReleasesStatusFilters(t *testing.T) {
	s := queryFixture(t)
	live := []string{"r-review", "r-matched", "r-removed", "r-new", "r-unmatched"}
	cases := []struct {
		status string
		want   []string
	}{
		{"", live},
		{"all", live},
		{"matched", []string{"r-matched"}},
		{"review", []string{"r-review"}},
		{"unmatched", []string{"r-new", "r-unmatched"}},
		{"removed", []string{"r-removed"}},
		{"new", []string{"r-new"}},
		{"ignored", []string{"r-ignored"}},
		{"something the ui never sends", live},
	}
	for _, tc := range cases {
		t.Run("status "+tc.status, func(t *testing.T) {
			page := s.QueryReleases(ReleaseQuery{Status: tc.status, Sort: "title"})
			if got := idsOf(page); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("ids = %v, want %v", got, tc.want)
			}
			if page.Total != len(tc.want) {
				t.Fatalf("total = %d, want %d", page.Total, len(tc.want))
			}
		})
	}
}

func TestQueryReleasesSortModes(t *testing.T) {
	s := queryFixture(t)
	cases := []struct {
		sort string
		want []string
	}{
		{"title", []string{"r-review", "r-matched", "r-removed", "r-new", "r-unmatched"}},
		{"size", []string{"r-new", "r-matched", "r-unmatched", "r-review", "r-removed"}},
		{"", []string{"r-review", "r-matched", "r-unmatched", "r-removed", "r-new"}},
		{"newest", []string{"r-review", "r-matched", "r-unmatched", "r-removed", "r-new"}},
	}
	for _, tc := range cases {
		t.Run("sort "+tc.sort, func(t *testing.T) {
			got := idsOf(s.QueryReleases(ReleaseQuery{Sort: tc.sort}))
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("ids = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestQueryReleasesPagingBounds(t *testing.T) {
	s := queryFixture(t)
	cases := []struct {
		name  string
		query ReleaseQuery
		items int
		page  int
		size  int
	}{
		{"defaults", ReleaseQuery{}, 5, 1, defaultPageSize},
		{"negative page and size", ReleaseQuery{Page: -3, PageSize: -1}, 5, 1, defaultPageSize},
		{"size above the cap", ReleaseQuery{PageSize: 10 * maxPageSize}, 5, 1, maxPageSize},
		{"second page", ReleaseQuery{Page: 2, PageSize: 2}, 2, 2, 2},
		{"last partial page", ReleaseQuery{Page: 3, PageSize: 2}, 1, 3, 2},
		{"page past the end", ReleaseQuery{Page: 99, PageSize: 2}, 0, 99, 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			page := s.QueryReleases(tc.query)
			if len(page.Items) != tc.items || page.Page != tc.page || page.PageSize != tc.size || page.Total != 5 {
				t.Fatalf("page = {items:%d page:%d size:%d total:%d}, want {items:%d page:%d size:%d total:5}",
					len(page.Items), page.Page, page.PageSize, page.Total, tc.items, tc.page, tc.size)
			}
		})
	}
}

func TestQueryReleasesFiltersBySourceAndSearch(t *testing.T) {
	s := queryFixture(t)
	other := availableRelease("r-other", "other", "Alpha Remastered", "alpha remastered", "", "2.0")
	s.sources = append(s.sources, &Source{ID: "other", Name: "Other", Enabled: true})
	s.releases["other"] = []*Release{other}

	if got := idsOf(s.QueryReleases(ReleaseQuery{SourceID: "other"})); !reflect.DeepEqual(got, []string{"r-other"}) {
		t.Fatalf("source filter ids = %v", got)
	}
	if got := idsOf(s.QueryReleases(ReleaseQuery{SourceID: "missing"})); len(got) != 0 {
		t.Fatalf("an unknown source returned %v", got)
	}
	got := idsOf(s.QueryReleases(ReleaseQuery{Search: "  ALPHA ", Sort: "title"}))
	if !reflect.DeepEqual(got, []string{"r-review", "r-other"}) {
		t.Fatalf("search ids = %v, want both Alpha releases, case and padding ignored", got)
	}
	if got := idsOf(s.QueryReleases(ReleaseQuery{Search: "no such title anywhere"})); len(got) != 0 {
		t.Fatalf("search for nothing returned %v", got)
	}
}

func TestSearchDownloadableOffersOnlyWhatCanStart(t *testing.T) {
	day := func(n int) *time.Time {
		at := time.Date(2026, 1, n, 0, 0, 0, 0, time.UTC)
		return &at
	}
	old := availableRelease("r-old", "src", "Celeste", "celeste", "", "1.0")
	old.UploadedAt = day(1)
	newer := availableRelease("r-new", "src", "Celeste", "celeste", "", "1.4")
	newer.UploadedAt = day(5)
	patch := availableRelease("r-patch", "src", "Celeste Update", "celeste update", "", "1.5")
	patch.Kind = KindPatch
	removed := availableRelease("r-removed", "src", "Celeste", "celeste", "", "1.0")
	removed.Availability = AvailabilityRemoved
	ignored := availableRelease("r-ignored", "src", "Celeste", "celeste", "", "1.0")
	ignored.Ignored = true
	linkless := availableRelease("r-nolink", "src", "Celeste", "celeste", "", "1.0")
	off := availableRelease("r-off", "off", "Celeste", "celeste", "", "1.0")
	other := availableRelease("r-other", "src", "Hades", "hades", "", "1.0")
	for _, r := range []*Release{old, newer, patch, removed, ignored, off, other} {
		r.URIs = []string{"magnet:?xt=urn:btih:" + r.ID}
	}
	s := searchService(t,
		[]*Source{{ID: "src", Name: "Feed", Enabled: true}, {ID: "off", Name: "Off", Enabled: false}},
		map[string][]*Release{"src": {old, newer, patch, removed, ignored, linkless, other}, "off": {off}},
	)
	ids := func(views []ReleaseView) []string {
		out := []string{}
		for _, v := range views {
			out = append(out, v.Release.ID)
		}
		return out
	}
	if got := ids(s.SearchDownloadable(" CELESTE ", 10)); !reflect.DeepEqual(got, []string{"r-new", "r-old"}) {
		t.Fatalf("ids = %v, want the two full, available releases newest first", got)
	}
	if got := s.SearchDownloadable("celeste", 1); len(got) != 1 || got[0].SourceName != "Feed" {
		t.Fatalf("limit/source name = %+v", got)
	}
	if got := s.SearchDownloadable("  ", 10); len(got) != 0 {
		t.Fatalf("empty query = %v", ids(got))
	}
	// A release id from elsewhere cannot reach what search hides.
	for _, id := range []string{"r-patch", "r-removed", "r-ignored", "r-nolink", "r-off", "missing"} {
		if _, err := s.PrepareDownloadable(id); err == nil {
			t.Errorf("%s was prepared", id)
		}
	}
	if req, err := s.PrepareDownloadable("r-new"); err != nil || req.ReleaseID != "r-new" || req.URI == "" {
		t.Fatalf("downloadable release: %+v %v", req, err)
	}
}

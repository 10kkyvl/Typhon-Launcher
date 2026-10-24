package sources

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"typhon/internal/catalog"
)

type candidateRemote struct {
	sourceMatchRemote
	answers map[string]catalog.ReleaseMatch
	calls   int
}

func (r *candidateRemote) MatchReleases(_ context.Context, qs []catalog.ReleaseQuery) ([]catalog.ReleaseMatch, error) {
	r.calls++
	out := make([]catalog.ReleaseMatch, len(qs))
	for i, q := range qs {
		for _, title := range q.Titles {
			for key, answer := range r.answers {
				if strings.Contains(strings.ToLower(title), key) {
					out[i] = answer
				}
			}
		}
	}
	return out, nil
}

type similarFixture struct {
	s       *Service
	cat     *catalog.Service
	dir     string
	src     Source
	remote  *candidateRemote
	one     catalog.Game
	two     catalog.Game
	family  catalog.Game
	foreign catalog.Game
}

func newSimilarFixture(t *testing.T) similarFixture {
	t.Helper()
	s, cat, dir := testService(t)
	f := similarFixture{s: s, cat: cat, dir: dir}
	var err error
	if f.one, err = cat.EnsureGame("Quillon One", 2010); err != nil {
		t.Fatal(err)
	}
	if f.two, err = cat.EnsureGame("Quillon Two", 2012); err != nil {
		t.Fatal(err)
	}
	if f.family, err = cat.EnsureGame("Quillon Remaster", 2020); err != nil {
		t.Fatal(err)
	}
	if f.foreign, err = cat.EnsureGame("Wombat Tycoon", 2001); err != nil {
		t.Fatal(err)
	}
	cand := func(g catalog.Game, year int) catalog.ReleaseCandidate {
		return catalog.ReleaseCandidate{ID: g.ID, Title: g.Title, ReleaseYear: year, Platforms: []string{"PC"}}
	}
	f.remote = &candidateRemote{answers: map[string]catalog.ReleaseMatch{
		"zorblax":  {Reason: catalog.ReasonAmbiguous, Candidates: []catalog.ReleaseCandidate{cand(f.one, 2010), cand(f.two, 2012)}},
		"yarrow":   {Reason: catalog.ReasonAmbiguous, Candidates: []catalog.ReleaseCandidate{cand(f.family, 2020), cand(f.two, 2012)}},
		"xylophon": {Reason: catalog.ReasonNone},
		"wistful":  {Reason: catalog.ReasonAmbiguous, Candidates: []catalog.ReleaseCandidate{cand(f.foreign, 2001)}},
	}}
	server := newFeedServer(t, feedBody(t, "Source",
		feedEntry{Title: "Zorblax Saga", URIs: []string{magnetOf("a1")}},
		feedEntry{Title: "Yarrow Chronicles", URIs: []string{magnetOf("a2")}},
		feedEntry{Title: "Xylophon Heroes", URIs: []string{magnetOf("a3")}},
		feedEntry{Title: "Wistful Wanderer", URIs: []string{magnetOf("a4")}},
	))
	f.src = addSource(t, s, server.url())
	cat.SetRemoteCatalog(f.remote)
	if _, err := s.RefreshSource(f.src.ID); err != nil {
		t.Fatal(err)
	}
	return f
}

func (f similarFixture) release(t *testing.T, s *Service, prefix string) ReleaseView {
	t.Helper()
	for _, v := range s.QueryReleases(ReleaseQuery{SourceID: f.src.ID, PageSize: maxPageSize}).Items {
		if strings.HasPrefix(v.Release.RawTitle, prefix) {
			return v
		}
	}
	t.Fatalf("no release %q", prefix)
	return ReleaseView{}
}

func TestAmbiguousAnswerBecomesReviewWithCachedCandidates(t *testing.T) {
	f := newSimilarFixture(t)
	zorblax := f.release(t, f.s, "Zorblax")
	if zorblax.Release.MatchStatus != catalog.StatusReview || zorblax.Release.CanonicalGameID != nil {
		t.Fatalf("ambiguous release: %+v", zorblax.Release)
	}
	rm := zorblax.Release.RemoteMatch
	want := []RemoteCandidate{
		{ID: f.one.ID, Title: "Quillon One", Year: 2010, Platforms: []string{"PC"}},
		{ID: f.two.ID, Title: "Quillon Two", Year: 2012, Platforms: []string{"PC"}},
	}
	if rm == nil || rm.Reason != catalog.ReasonAmbiguous || !reflect.DeepEqual(rm.Candidates, want) {
		t.Fatalf("cache: %+v", rm)
	}
	none := f.release(t, f.s, "Xylophon")
	if none.Release.MatchStatus != catalog.StatusUnmatched || none.Release.RemoteMatch.Reason != catalog.ReasonNone || len(none.Release.RemoteMatch.Candidates) != 0 {
		t.Fatalf("none release: %+v / %+v", none.Release, none.Release.RemoteMatch)
	}
	src, err := f.s.GetSource(f.src.ID)
	if err != nil || src.Review != 3 || src.Unmatched != 1 || src.Matched != 0 {
		t.Fatalf("counters: %+v %v", src, err)
	}
}

func TestCandidatesSurviveRestart(t *testing.T) {
	f := newSimilarFixture(t)
	reloaded := mustServiceAt(t, f.dir, mustCatalog(t, f.dir))
	got := f.release(t, reloaded, "Zorblax")
	if got.Release.MatchStatus != catalog.StatusReview || len(got.Release.RemoteMatch.Candidates) != 2 || got.Release.RemoteMatch.Reason != catalog.ReasonAmbiguous {
		t.Fatalf("reloaded: %+v / %+v", got.Release, got.Release.RemoteMatch)
	}
}

func TestGetCandidatesPutsServerFirstWithoutDuplicates(t *testing.T) {
	f := newSimilarFixture(t)
	zorblax := f.release(t, f.s, "Zorblax")
	list, err := f.s.GetCandidates(zorblax.Release.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) < 2 || list[0].GameID != f.one.ID || list[1].GameID != f.two.ID {
		t.Fatalf("server candidates not first: %+v", list)
	}
	seen := map[string]bool{}
	for i, c := range list {
		if seen[c.GameID] {
			t.Fatalf("duplicate %s: %+v", c.GameID, list)
		}
		seen[c.GameID] = true
		wantSource := catalog.SourceLocal
		if i < 2 {
			wantSource = catalog.SourceServer
		}
		if c.Source != wantSource || !c.InCatalog {
			t.Fatalf("candidate %d: %+v", i, c)
		}
	}
	if list[0].Year == nil || *list[0].Year != 2010 || !reflect.DeepEqual(list[0].Platforms, []string{"PC"}) {
		t.Fatalf("first candidate: %+v", list[0])
	}
}

func TestSimilarReleases(t *testing.T) {
	f := newSimilarFixture(t)
	zorblax := f.release(t, f.s, "Zorblax")
	yarrow := f.release(t, f.s, "Yarrow")
	cases := []struct {
		name   string
		game   string
		family []string
		want   []string
		direct []string
	}{
		{"direct candidate", f.one.ID, nil, []string{zorblax.Release.ID}, []string{f.one.ID}},
		{"both direct", f.two.ID, nil, sortedByDate(zorblax, yarrow), []string{f.two.ID, f.two.ID}},
		{"direct first then family", f.two.ID, []string{f.family.ID}, sortedByDate(zorblax, yarrow), []string{f.two.ID, f.two.ID}},
		{"family only", f.one.ID, []string{f.family.ID}, []string{zorblax.Release.ID, yarrow.Release.ID}, []string{f.one.ID, f.family.ID}},
		{"nothing for a foreign game", "other", []string{"another"}, nil, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := f.s.SimilarReleases(tc.game, tc.family)
			if err != nil {
				t.Fatal(err)
			}
			var ids, matched []string
			for _, item := range got {
				ids = append(ids, item.Release.ID)
				matched = append(matched, item.MatchedCandidate.ID)
				if item.Reason != catalog.ReasonAmbiguous || item.MatchedCandidate.Title == "" {
					t.Fatalf("view: %+v", item)
				}
			}
			if !reflect.DeepEqual(ids, tc.want) {
				t.Fatalf("ids = %v, want %v", ids, tc.want)
			}
			if !reflect.DeepEqual(matched, tc.direct) {
				t.Fatalf("matched candidates = %v, want %v", matched, tc.direct)
			}
		})
	}
}

func sortedByDate(a, b ReleaseView) []string {
	list := []*Release{&a.Release, &b.Release}
	sortReleases(list, "")
	return []string{list[0].ID, list[1].ID}
}

func TestSimilarReleasesPutsDirectCandidateBeforeFamily(t *testing.T) {
	f := newSimilarFixture(t)
	zorblax := f.release(t, f.s, "Zorblax")
	yarrow := f.release(t, f.s, "Yarrow")
	got, err := f.s.SimilarReleases(f.family.ID, []string{f.one.ID})
	if err != nil || len(got) != 2 || got[0].Release.ID != yarrow.Release.ID || got[1].Release.ID != zorblax.Release.ID {
		t.Fatalf("order: %+v %v", got, err)
	}
}

func TestSimilarReleasesSkipsIgnoredRemovedAndMatched(t *testing.T) {
	f := newSimilarFixture(t)
	zorblax := f.release(t, f.s, "Zorblax")
	yarrow := f.release(t, f.s, "Yarrow")
	if err := f.s.IgnoreRelease(zorblax.Release.ID, true); err != nil {
		t.Fatal(err)
	}
	got, err := f.s.SimilarReleases(f.two.ID, nil)
	if err != nil || len(got) != 1 || got[0].Release.ID != yarrow.Release.ID {
		t.Fatalf("ignored not skipped: %+v %v", got, err)
	}
	f.s.mu.Lock()
	f.s.findReleaseLocked(yarrow.Release.ID).Availability = AvailabilityRemoved
	f.s.mu.Unlock()
	if got, err = f.s.SimilarReleases(f.two.ID, nil); err != nil || len(got) != 0 {
		t.Fatalf("removed not skipped: %+v %v", got, err)
	}
}

func TestSimilarReleasesRejectsBadArguments(t *testing.T) {
	s, _, _ := testService(t)
	tooMany := make([]string, maxFamilyIDs+1)
	for i := range tooMany {
		tooMany[i] = "g"
	}
	cases := []struct {
		name   string
		game   string
		family []string
	}{
		{"empty game", "", nil},
		{"empty family id", "g", []string{"a", ""}},
		{"too many family ids", "g", tooMany},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got, err := s.SimilarReleases(tc.game, tc.family); err == nil || got != nil {
				t.Fatalf("got %+v, %v", got, err)
			}
		})
	}
}

func TestConfirmMatchTakesReviewReleaseOutOfSimilar(t *testing.T) {
	f := newSimilarFixture(t)
	zorblax := f.release(t, f.s, "Zorblax")
	callsBefore := f.remote.calls
	if err := f.s.ConfirmMatch(zorblax.Release.ID, f.one.ID); err != nil {
		t.Fatal(err)
	}
	if f.remote.calls != callsBefore {
		t.Fatalf("confirm asked the server: %d calls", f.remote.calls-callsBefore)
	}
	got, err := f.s.SimilarReleases(f.one.ID, nil)
	if err != nil || len(got) != 0 {
		t.Fatalf("still similar: %+v %v", got, err)
	}
	groups := f.s.GetReleasesForGame(f.one.ID)
	if len(groups) != 1 || groups[0].Release.ID != zorblax.Release.ID || groups[0].Release.MatchStatus != catalog.StatusMatched || !groups[0].Release.Locked {
		t.Fatalf("not attached: %+v", groups)
	}
}

func TestOldServerAnswerWithoutFieldsStaysUnmatched(t *testing.T) {
	s, cat, _ := testService(t)
	server := newFeedServer(t, feedBody(t, "Source", feedEntry{Title: "Plainly Unknown", URIs: []string{magnetOf("b1")}}))
	src := addSource(t, s, server.url())
	cat.SetRemoteCatalog(&sourceMatchRemote{})
	if _, err := s.RefreshSource(src.ID); err != nil {
		t.Fatal(err)
	}
	items := releasesOf(t, s, src.ID, "")
	if len(items) != 1 || items[0].Release.MatchStatus != catalog.StatusUnmatched {
		t.Fatalf("release: %+v", items)
	}
	if rm := items[0].Release.RemoteMatch; rm == nil || rm.Reason != "" || len(rm.Candidates) != 0 {
		t.Fatalf("cache: %+v", rm)
	}
}

package catalog

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"testing"
)

type consoleSecondPass struct {
	first ReleaseMatch
	err   error
	calls int
}

func (f *consoleSecondPass) Browse(context.Context, GameQuery) (GamePage, error) {
	return GamePage{}, nil
}

func (f *consoleSecondPass) MatchReleases(_ context.Context, qs []ReleaseQuery) ([]ReleaseMatch, error) {
	f.calls++
	if f.calls == 1 {
		return []ReleaseMatch{f.first}, nil
	}
	return make([]ReleaseMatch, len(qs)), f.err
}

func TestConsoleSecondPassFailure(t *testing.T) {
	accept := func(platforms []string) bool { return len(platforms) == 1 && platforms[0] == "PlayStation 2" }
	ambiguous := ReleaseMatch{Reason: ReasonAmbiguous, Candidates: []ReleaseCandidate{
		{ID: "a", Title: "Alpha", Platforms: []string{"PC"}},
		{ID: "b", Title: "Alpha", Platforms: []string{"PlayStation 2"}},
	}}
	boom := errors.New("boom")
	cases := []struct {
		name    string
		err     error
		wantErr error
		status  Status
	}{
		{"cancelled", context.Canceled, context.Canceled, ""},
		{"server failure keeps the first pass", boom, nil, StatusReview},
		{"outdated server keeps the first pass", ErrBackendOutdated, nil, StatusReview},
		{"partial answer keeps the candidate", &PartialMatchError{Failed: []int{0}, Total: 1, Err: boom}, nil, StatusReview},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := mustServiceAt(t, t.TempDir())
			s.SetRemoteCatalog(&consoleSecondPass{first: ambiguous, err: tc.err})
			out, err := s.ResolveSourceQueries(context.Background(), []ReleaseQuery{{Titles: []string{"Alpha"}, Accept: accept}})
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
			if tc.wantErr != nil {
				return
			}
			if out[0].Status != tc.status || len(out[0].Candidates) != 1 || out[0].Candidates[0].GameID != "b" {
				t.Fatalf("match: %+v", out[0])
			}
		})
	}
}

type consoleYearRemote struct {
	first ReleaseMatch
	calls int
}

func (f *consoleYearRemote) Browse(context.Context, GameQuery) (GamePage, error) {
	return GamePage{}, nil
}

func (f *consoleYearRemote) MatchReleases(_ context.Context, qs []ReleaseQuery) ([]ReleaseMatch, error) {
	f.calls++
	if f.calls == 1 {
		return []ReleaseMatch{f.first}, nil
	}
	out := make([]ReleaseMatch, len(qs))
	for i, q := range qs {
		id := "ps2-" + strconv.Itoa(q.Year)
		out[i] = ReleaseMatch{Game: &Game{ID: id, Title: q.Titles[0], Platforms: []string{"PlayStation 2"}, ExternalIDs: ExternalIDs{IGDB: id}}}
	}
	return out, nil
}

func TestConsoleCandidateYearNarrowsSeveral(t *testing.T) {
	accept := func(platforms []string) bool { return slices.Contains(platforms, "PlayStation 2") }
	record := func(id string, year int) ReleaseCandidate {
		return ReleaseCandidate{ID: id, Title: "Alpha", ReleaseYear: year, Platforms: []string{"PlayStation 2"}}
	}
	other := record("other-2010", 2010)
	other.Title = "Alpha Reissue"
	dropped := []ReleaseCandidate{record("ps2-2005", 2005), record("ps2-2010", 2010), other, record("ps2-0", 0)}
	cases := []struct {
		name   string
		year   int
		status Status
		game   string
		review []string
	}{
		{"the year names one", 2005, StatusMatched, "ps2-2005", nil},
		{"the year names two", 2010, StatusReview, "", []string{"ps2-2010", "other-2010"}},
		{"the year names none", 1999, StatusReview, "", []string{"ps2-2005", "ps2-2010", "other-2010"}},
		{"no year", 0, StatusReview, "", []string{"ps2-2005", "ps2-2010", "other-2010"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := mustServiceAt(t, t.TempDir())
			s.SetRemoteCatalog(&consoleYearRemote{first: ReleaseMatch{
				Game:              &Game{ID: "pc", Title: "Alpha", Platforms: []string{"PC"}, ExternalIDs: ExternalIDs{IGDB: "pc"}},
				ConsoleCandidates: dropped,
			}})
			out, err := s.ResolveSourceQueries(context.Background(), []ReleaseQuery{{Titles: []string{"Alpha"}, Year: tc.year, Accept: accept}})
			if err != nil {
				t.Fatal(err)
			}
			if out[0].Status != tc.status {
				t.Fatalf("status = %q, want %q: %+v", out[0].Status, tc.status, out[0])
			}
			if tc.status == StatusMatched {
				game, err := s.GetGame(out[0].GameID)
				if err != nil || game.ServerID != tc.game {
					t.Fatalf("game %+v (%v), want server id %q", game, err, tc.game)
				}
				return
			}
			ids := []string{}
			for _, c := range out[0].Candidates {
				ids = append(ids, c.GameID)
			}
			if !slices.Equal(ids, tc.review) {
				t.Fatalf("candidates = %q, want %q", ids, tc.review)
			}
		})
	}
}

func TestConsoleCandidateWithProviderIDBecomesTheGame(t *testing.T) {
	accept := func(platforms []string) bool { return slices.Contains(platforms, "PlayStation 2") }
	year := 2005
	full := ReleaseCandidate{
		ID: "ps2", Title: "Alpha", ReleaseYear: year, Platforms: []string{"PlayStation 2"}, GameType: "Main Game", ParentID: "base",
		ExternalIDs: ExternalIDs{IGDB: "2", Steam: "20"}, CoverURL: "https://cover.example/ps2.jpg",
	}
	badCover := full
	badCover.CoverURL = "http://cover.example/ps2.jpg"
	steamOnly := full
	steamOnly.ExternalIDs = ExternalIDs{Steam: "20"}
	namesakes := []struct {
		name  string
		pc    Game
		cand  ReleaseCandidate
		cover string
		ids   ExternalIDs
	}{
		{"pc a year later", Game{GameType: "Main Game", ReleaseYear: new(2006)}, full, full.CoverURL, full.ExternalIDs},
		{"pc a year earlier", Game{GameType: "Main Game", ReleaseYear: new(2004)}, full, full.CoverURL, full.ExternalIDs},
		{"pc port", Game{GameType: "Port", ReleaseYear: new(2004)}, full, full.CoverURL, full.ExternalIDs},
		{"unusable cover keeps the match", Game{GameType: "Main Game", ReleaseYear: new(2006)}, badCover, "", full.ExternalIDs},
		{"steam id alone is enough", Game{GameType: "Main Game", ReleaseYear: new(2006)}, steamOnly, full.CoverURL, steamOnly.ExternalIDs},
	}
	for _, tc := range namesakes {
		t.Run(tc.name, func(t *testing.T) {
			pc := tc.pc
			pc.ID, pc.Title, pc.Platforms, pc.ExternalIDs = "pc", "Alpha", []string{"PC"}, ExternalIDs{IGDB: "pc"}
			s := mustServiceAt(t, t.TempDir())
			remote := &consoleYearRemote{first: ReleaseMatch{Game: &pc, ConsoleCandidates: []ReleaseCandidate{tc.cand}}}
			s.SetRemoteCatalog(remote)
			out, err := s.ResolveSourceQueries(context.Background(), []ReleaseQuery{{Titles: []string{"Alpha"}, Year: year, Accept: accept}})
			if err != nil {
				t.Fatal(err)
			}
			if out[0].Status != StatusMatched || remote.calls != 1 {
				t.Fatalf("match %+v after %d calls, want matched after one", out[0], remote.calls)
			}
			game, err := s.GetGame(out[0].GameID)
			if err != nil {
				t.Fatal(err)
			}
			if game.ServerID != "ps2" || game.ExternalIDs != tc.ids || game.CoverURL != tc.cover || game.GameType != "Main Game" || game.ReleaseYear == nil || *game.ReleaseYear != year || !accept(game.Platforms) {
				t.Fatalf("game: %+v", game)
			}
		})
	}
}

func TestConsoleCandidateWithoutUsableIdentityIsAskedAgain(t *testing.T) {
	accept := func(platforms []string) bool { return slices.Contains(platforms, "PlayStation 2") }
	cases := []struct {
		name string
		cand ReleaseCandidate
	}{
		{"no provider id", ReleaseCandidate{ID: "ps2-2005", Title: "Alpha", ReleaseYear: 2005, Platforms: []string{"PlayStation 2"}}},
		{"blank provider id", ReleaseCandidate{ID: "ps2-2005", Title: "Alpha", ReleaseYear: 2005, Platforms: []string{"PlayStation 2"}, ExternalIDs: ExternalIDs{IGDB: " \t"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := mustServiceAt(t, t.TempDir())
			remote := &consoleYearRemote{first: ReleaseMatch{
				Game:              &Game{ID: "pc", Title: "Alpha", Platforms: []string{"PC"}, ExternalIDs: ExternalIDs{IGDB: "pc"}},
				ConsoleCandidates: []ReleaseCandidate{tc.cand},
			}}
			s.SetRemoteCatalog(remote)
			out, err := s.ResolveSourceQueries(context.Background(), []ReleaseQuery{{Titles: []string{"Alpha"}, Accept: accept}})
			if err != nil {
				t.Fatal(err)
			}
			if out[0].Status != StatusMatched || remote.calls != 2 {
				t.Fatalf("match %+v after %d calls, want the second pass", out[0], remote.calls)
			}
		})
	}
}

func TestConsoleCandidateCopyWithProviderIDWins(t *testing.T) {
	accept := func(platforms []string) bool { return slices.Contains(platforms, "PlayStation 2") }
	bare := ReleaseCandidate{ID: "ps2", Title: "Alpha", Platforms: []string{"PlayStation 2"}}
	full := bare
	full.ExternalIDs = ExternalIDs{IGDB: "2"}
	remote := &consoleYearRemote{first: ReleaseMatch{Reason: ReasonAmbiguous, Candidates: []ReleaseCandidate{bare}, ConsoleCandidates: []ReleaseCandidate{full}}}
	s := mustServiceAt(t, t.TempDir())
	s.SetRemoteCatalog(remote)
	out, err := s.ResolveSourceQueries(context.Background(), []ReleaseQuery{{Titles: []string{"Alpha"}, Accept: accept}})
	if err != nil {
		t.Fatal(err)
	}
	if out[0].Status != StatusMatched || remote.calls != 1 {
		t.Fatalf("match %+v after %d calls", out[0], remote.calls)
	}
}

func TestConsoleCandidatesIgnoredForPCRelease(t *testing.T) {
	s := mustServiceAt(t, t.TempDir())
	remote := &consoleYearRemote{first: ReleaseMatch{
		Game:              &Game{ID: "pc", Title: "Alpha", Platforms: []string{"PC"}, ExternalIDs: ExternalIDs{IGDB: "pc"}},
		ConsoleCandidates: []ReleaseCandidate{{ID: "ps2", Title: "Alpha", Platforms: []string{"PlayStation 2"}}},
	}}
	s.SetRemoteCatalog(remote)
	out, err := s.ResolveSourceQueries(context.Background(), []ReleaseQuery{{Titles: []string{"Alpha"}}})
	if err != nil {
		t.Fatal(err)
	}
	if out[0].Status != StatusMatched || remote.calls != 1 {
		t.Fatalf("match %+v after %d calls", out[0], remote.calls)
	}
}

func TestConsoleMatchFillsMissingPlatformsOfSavedGame(t *testing.T) {
	s := mustServiceAt(t, t.TempDir())
	if _, err := s.AddGame(Game{ID: "local", ServerID: "srv", Title: "Alpha", ExternalIDs: ExternalIDs{IGDB: "1"}}); err != nil {
		t.Fatal(err)
	}
	platforms := []string{"PlayStation 2"}
	s.SetRemoteCatalog(&consoleSecondPass{first: ReleaseMatch{Game: &Game{ID: "srv", Title: "Alpha", Platforms: platforms, ExternalIDs: ExternalIDs{IGDB: "1"}}}})
	accept := func(got []string) bool { return len(got) == 1 && got[0] == platforms[0] }
	if _, err := s.ResolveSourceQueries(context.Background(), []ReleaseQuery{{Titles: []string{"Alpha"}, Accept: accept}}); err != nil {
		t.Fatal(err)
	}
	game, err := s.GetGame("local")
	if err != nil {
		t.Fatal(err)
	}
	if !accept(game.Platforms) {
		t.Fatalf("platforms = %q", game.Platforms)
	}
}

func TestConsoleDuplicateRecordsTakeTheServerFirst(t *testing.T) {
	accept := func(platforms []string) bool { return slices.Contains(platforms, "PlayStation") }
	record := func(id, title string, year int) ReleaseCandidate {
		return ReleaseCandidate{ID: id, Title: title, ReleaseYear: year, Platforms: []string{"PlayStation"}, ExternalIDs: ExternalIDs{IGDB: id}}
	}
	bare := func(id string) ReleaseCandidate {
		return ReleaseCandidate{ID: id, Title: "Alpha", ReleaseYear: 1997, Platforms: []string{"PlayStation"}}
	}
	cases := []struct {
		name    string
		records []ReleaseCandidate
		game    string
	}{
		{"same title and year", []ReleaseCandidate{record("1", "Alpha", 1997), record("2", "ALPHA", 1997)}, "1"},
		{"different years", []ReleaseCandidate{record("1", "Alpha", 1997), record("2", "Alpha", 1999)}, ""},
		{"different titles", []ReleaseCandidate{record("1", "Alpha", 1997), record("2", "Alpha Reissue", 1997)}, ""},
		{"unknown year", []ReleaseCandidate{record("1", "Alpha", 0), record("2", "Alpha", 0)}, ""},
		{"no provider ids", []ReleaseCandidate{bare("1"), bare("2")}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := mustServiceAt(t, t.TempDir())
			s.SetRemoteCatalog(&consoleYearRemote{first: ReleaseMatch{Reason: ReasonAmbiguous, Candidates: tc.records}})
			out, err := s.ResolveSourceQueries(context.Background(), []ReleaseQuery{{Titles: []string{"Alpha"}, Accept: accept}})
			if err != nil {
				t.Fatal(err)
			}
			if tc.game == "" {
				if out[0].Status != StatusReview || len(out[0].Candidates) != 2 {
					t.Fatalf("result = %+v, want review with both records", out[0])
				}
				return
			}
			game, err := s.GetGame(out[0].GameID)
			if out[0].Status != StatusMatched || err != nil || game.ExternalIDs.IGDB != tc.game {
				t.Fatalf("result %+v, game %+v (%v), want igdb %q", out[0], game, err, tc.game)
			}
		})
	}
}

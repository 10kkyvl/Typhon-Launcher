package catalog

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"typhon/internal/titles"
)

func newTestService(t *testing.T) *Service {
	t.Helper()
	return mustServiceAt(t, t.TempDir())
}

func mustServiceAt(t *testing.T, dir string) *Service {
	t.Helper()
	s, err := NewServiceAt(dir)
	if err != nil {
		t.Fatalf("new catalog service at %s: %v", dir, err)
	}
	return s
}

func year(v int) *int { return &v }

func seed(t *testing.T, s *Service, games ...Game) []Game {
	t.Helper()
	out := make([]Game, 0, len(games))
	for _, g := range games {
		added, err := s.AddGame(g)
		if err != nil {
			t.Fatalf("add game %q: %v", g.Title, err)
		}
		out = append(out, added)
	}
	return out
}

func query(title string) Query {
	parsed := titles.Parse(title)
	return Query{Title: parsed.Base, Normalized: parsed.Normalized, Year: parsed.Year}
}

func TestResolveExactTitle(t *testing.T) {
	s := newTestService(t)
	games := seed(t, s,
		Game{Title: "Cyberpunk 2077", ReleaseYear: year(2020)},
		Game{Title: "The Witcher 3 Wild Hunt", ReleaseYear: year(2015)},
	)

	match := s.Resolve(query("Cyberpunk.2077.Ultimate.Edition.v2.31.MULTi19"))
	if match.Status != StatusMatched {
		t.Fatalf("status = %s, want matched", match.Status)
	}
	if match.GameID != games[0].ID {
		t.Fatalf("game = %s, want %s", match.GameID, games[0].ID)
	}
	if match.Confidence < AutoThreshold {
		t.Fatalf("confidence = %f, want >= %f", match.Confidence, AutoThreshold)
	}
}

func TestResolveAlias(t *testing.T) {
	s := newTestService(t)
	games := seed(t, s, Game{Title: "Cyberpunk 2077", Aliases: []string{"CP2077"}})

	match := s.Resolve(query("CP2077"))
	if match.Status != StatusMatched || match.GameID != games[0].ID {
		t.Fatalf("match = %+v, want matched to %s", match, games[0].ID)
	}
	if match.Method != MethodAlias {
		t.Fatalf("method = %s, want alias", match.Method)
	}
}

func TestResolveAmbiguousTitleNeedsReview(t *testing.T) {
	s := newTestService(t)
	seed(t, s,
		Game{Title: "Prey", ReleaseYear: year(2006)},
		Game{Title: "Prey", ReleaseYear: year(2017)},
	)

	match := s.Resolve(query("Prey.v1.0.MULTi9"))
	if match.Status != StatusReview {
		t.Fatalf("status = %s, want review", match.Status)
	}
	if match.GameID != "" {
		t.Fatalf("game = %s, want empty for ambiguous match", match.GameID)
	}
	if len(match.Candidates) < 2 {
		t.Fatalf("candidates = %d, want at least 2", len(match.Candidates))
	}
}

func TestResolveAmbiguousResolvedByYear(t *testing.T) {
	s := newTestService(t)
	games := seed(t, s,
		Game{Title: "Prey", ReleaseYear: year(2006)},
		Game{Title: "Prey", ReleaseYear: year(2017)},
	)

	match := s.Resolve(query("Prey (2017) [MULTi9] v1.0"))
	if match.Status != StatusMatched {
		t.Fatalf("status = %s, want matched", match.Status)
	}
	if match.GameID != games[1].ID {
		t.Fatalf("game = %s, want the 2017 entry %s", match.GameID, games[1].ID)
	}
}

func TestResolveUnmatched(t *testing.T) {
	s := newTestService(t)
	seed(t, s, Game{Title: "Cyberpunk 2077"})

	match := s.Resolve(query("Totally Unrelated Puzzle Game"))
	if match.Status != StatusUnmatched {
		t.Fatalf("status = %s, want unmatched", match.Status)
	}
	if match.GameID != "" {
		t.Fatalf("game = %s, want empty", match.GameID)
	}
}

func TestResolveExternalID(t *testing.T) {
	s := newTestService(t)
	games := seed(t, s, Game{Title: "Half-Life 2", ExternalIDs: ExternalIDs{Steam: "220"}})

	match := s.Resolve(Query{Title: "Ничего похожего", ExternalIDs: ExternalIDs{Steam: "220"}})
	if match.Status != StatusMatched || match.GameID != games[0].ID {
		t.Fatalf("match = %+v, want matched to %s", match, games[0].ID)
	}
	if match.Confidence != scoreExternalID {
		t.Fatalf("confidence = %f, want %f", match.Confidence, scoreExternalID)
	}
}

func TestLearnMatchAppliesToNextResolve(t *testing.T) {
	s := newTestService(t)
	games := seed(t, s, Game{Title: "Cyberpunk 2077"})

	raw := "CP2077 Ultimate v2.31"
	before := s.Resolve(query(raw))
	if before.Status == StatusMatched {
		t.Fatalf("expected no automatic match for %q, got %+v", raw, before)
	}

	normalized := titles.Parse(raw).Normalized
	if err := s.LearnMatch(normalized, games[0].ID); err != nil {
		t.Fatalf("learn match: %v", err)
	}

	after := s.Resolve(query(raw))
	if after.Status != StatusMatched || after.GameID != games[0].ID {
		t.Fatalf("match = %+v, want matched to %s", after, games[0].ID)
	}
	if after.Method != MethodOverride {
		t.Fatalf("method = %s, want override", after.Method)
	}
}

func TestLearnMatchSurvivesRestart(t *testing.T) {
	dir := t.TempDir()
	s := mustServiceAt(t, dir)
	games := seed(t, s, Game{Title: "Cyberpunk 2077"})
	normalized := titles.Parse("CP2077 Ultimate v2.31").Normalized
	if err := s.LearnMatch(normalized, games[0].ID); err != nil {
		t.Fatalf("learn match: %v", err)
	}

	restarted := mustServiceAt(t, dir)
	match := restarted.Resolve(query("CP2077 Ultimate v2.31"))
	if match.Status != StatusMatched || match.GameID != games[0].ID {
		t.Fatalf("match after restart = %+v, want matched to %s", match, games[0].ID)
	}
	if len(restarted.ListGames()) != 1 {
		t.Fatalf("games after restart = %d, want 1", len(restarted.ListGames()))
	}
}

func TestProvisionCreatesOncePerTitle(t *testing.T) {
	s := newTestService(t)
	queries := []Query{
		query("Hades.II.v0.9"),
		query("Hades II Early Access"),
		query("Stardew.Valley.v1.6"),
	}
	provisioned := s.Provision(queries)
	if len(provisioned) != 3 {
		t.Fatalf("provisioned keys = %d, want 3", len(provisioned))
	}

	games := s.ListGames()
	if len(games) != 3 {
		t.Fatalf("games = %d, want 3", len(games))
	}
	for _, g := range games {
		if !g.Provisional {
			t.Fatalf("game %q should be provisional", g.Title)
		}
	}

	again := s.Provision(queries)
	if len(s.ListGames()) != 3 {
		t.Fatalf("games after second provision = %d, want 3", len(s.ListGames()))
	}
	if again[queries[0].Normalized].ID != provisioned[queries[0].Normalized].ID {
		t.Fatal("provision should reuse existing game")
	}
}

func TestProvisionReusesExistingCatalogGame(t *testing.T) {
	s := newTestService(t)
	games := seed(t, s, Game{Title: "Cyberpunk 2077"})

	provisioned := s.Provision([]Query{query("Cyberpunk 2077")})
	if got := provisioned[titles.Normalize("Cyberpunk 2077")]; got.ID != games[0].ID {
		t.Fatalf("provisioned %s, want existing %s", got.ID, games[0].ID)
	}
	if len(s.ListGames()) != 1 {
		t.Fatalf("games = %d, want 1", len(s.ListGames()))
	}
}

func TestSearchGames(t *testing.T) {
	s := newTestService(t)
	seed(t, s,
		Game{Title: "Cyberpunk 2077"},
		Game{Title: "The Witcher 3 Wild Hunt"},
		Game{Title: "Ultimate Chicken Horse"},
	)

	found := s.SearchGames("witcher", 5)
	if len(found) == 0 || found[0].Title != "The Witcher 3 Wild Hunt" {
		t.Fatalf("search result = %+v", found)
	}
	if len(s.SearchGames("", 5)) != 0 {
		t.Fatal("empty query should return nothing")
	}
}

func TestPersistedCatalogFileIsVersioned(t *testing.T) {
	dir := t.TempDir()
	s := mustServiceAt(t, dir)
	seed(t, s, Game{Title: "Cyberpunk 2077"})

	data, err := os.ReadFile(filepath.Join(dir, "catalog.json"))
	if err != nil {
		t.Fatalf("read catalog: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, `"version"`) || !strings.Contains(content, `"data"`) || !strings.Contains(content, "Cyberpunk 2077") {
		t.Fatalf("unexpected catalog file: %s", data)
	}
}

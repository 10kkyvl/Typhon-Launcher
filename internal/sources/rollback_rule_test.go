package sources

import (
	"os"
	"path/filepath"
	"testing"

	"typhon/internal/catalog"
)

func rulesOf(c *catalog.Service) map[string]string {
	out := map[string]string{}
	for _, o := range c.ListOverrides() {
		out[o.Pattern] = o.GameID
	}
	return out
}

func TestConfirmMatchTakesTheRuleBackWhenTheReleasesCannotBeSaved(t *testing.T) {
	tests := []struct {
		name         string
		previousRule bool
	}{
		{"no rule before", false},
		{"a rule for another game before", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			catDir := t.TempDir()
			cat := mustCatalog(t, catDir)
			oldGame, err := cat.AddGame(catalog.Game{Title: "Cyberpunk 2077 Ultimate"})
			if err != nil {
				t.Fatal(err)
			}
			game, err := cat.AddGame(catalog.Game{Title: "Cyberpunk 2077"})
			if err != nil {
				t.Fatal(err)
			}
			s := mustServiceAt(t, dir, cat)
			server := newFeedServer(t, feedBody(t, "Feed", feedEntry{Title: "CP2077 Ultimate v2.31", URIs: []string{magnetOf("aa")}}))
			src := addSource(t, s, server.url())
			items := releasesOf(t, s, src.ID, "all")
			if len(items) != 1 {
				t.Fatalf("releases = %d", len(items))
			}
			release := items[0].Release
			want := map[string]string{}
			if tt.previousRule {
				if _, err := cat.LearnMatch(release.NormalizedTitle, oldGame.ID); err != nil {
					t.Fatal(err)
				}
				want[release.NormalizedTitle] = oldGame.ID
			}
			occupyDir(t, dir)

			if err := s.ConfirmMatch(release.ID, game.ID); err == nil {
				t.Fatal("expected the save failure")
			}

			for name, view := range map[string]*catalog.Service{"memory": cat, "disk": mustCatalog(t, catDir)} {
				got := rulesOf(view)
				if len(got) != len(want) || got[release.NormalizedTitle] != want[release.NormalizedTitle] {
					t.Errorf("%s: rules = %v, want %v: a match that was never saved left its rule behind", name, got, want)
				}
			}
			stored, err := cat.GetGame(game.ID)
			if err != nil {
				t.Fatal(err)
			}
			if len(stored.Aliases) != 0 {
				t.Errorf("aliases = %v, want the alias of the refused match removed", stored.Aliases)
			}
		})
	}
}

func TestConfirmMatchKeepsTheRuleOnceTheReleasesAreSaved(t *testing.T) {
	catDir := t.TempDir()
	cat := mustCatalog(t, catDir)
	game, err := cat.AddGame(catalog.Game{Title: "Cyberpunk 2077"})
	if err != nil {
		t.Fatal(err)
	}
	s := mustServiceAt(t, t.TempDir(), cat)
	server := newFeedServer(t, feedBody(t, "Feed", feedEntry{Title: "CP2077 Ultimate v2.31", URIs: []string{magnetOf("aa")}}))
	src := addSource(t, s, server.url())
	release := releasesOf(t, s, src.ID, "all")[0].Release

	if err := s.ConfirmMatch(release.ID, game.ID); err != nil {
		t.Fatal(err)
	}

	if got := rulesOf(mustCatalog(t, catDir))[release.NormalizedTitle]; got != game.ID {
		t.Fatalf("rule = %q, want %q", got, game.ID)
	}
}

func TestRemoveSourceKeepsTheSourceWhenItsReleasesCannotBeRemoved(t *testing.T) {
	t.Parallel()
	s, _, src, dir := blockedService(t)
	releasesFile := filepath.Join(dir, "releases", src.ID+".json")
	want := len(releasesOf(t, s, src.ID, "all"))
	if err := os.Remove(releasesFile); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(releasesFile, "held"), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := s.RemoveSource(src.ID); err == nil {
		t.Fatal("RemoveSource() error = nil, want the releases file that stayed behind reported")
	}

	if _, err := s.GetSource(src.ID); err != nil {
		t.Fatalf("the source is gone from memory although its releases are still on disk: %v", err)
	}
	if _, ok := storedSource(t, s, src.ID); !ok {
		t.Fatal("sources.json lost a source that was not removed")
	}
	if got := len(releasesOf(t, s, src.ID, "all")); got != want {
		t.Fatalf("releases in memory = %d, want %d", got, want)
	}
}

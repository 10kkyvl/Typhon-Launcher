package catalog

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func overridesOf(s *Service) map[string]string {
	out := map[string]string{}
	for _, o := range s.ListOverrides() {
		out[o.Pattern] = o.GameID
	}
	return out
}

func aliasesOf(t *testing.T, s *Service, id string) []string {
	t.Helper()
	game, err := s.GetGame(id)
	if err != nil {
		t.Fatal(err)
	}
	return game.Aliases
}

func TestUndoMatchRestoresWhatLearnMatchReplaced(t *testing.T) {
	const pattern = "cyberpunk 2077 ultimate"
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
			s := mustServiceAt(t, dir)
			games := seed(t, s, Game{Title: "Cyberpunk 2077"}, Game{Title: "Phantom Liberty"})
			oldGame, newGame := games[0], games[1]
			wantRules := map[string]string{}
			var wantOldAliases []string
			if tt.previousRule {
				if _, err := s.LearnMatch(pattern, oldGame.ID); err != nil {
					t.Fatal(err)
				}
				wantRules[pattern] = oldGame.ID
				wantOldAliases = []string{pattern}
			}

			undo, err := s.LearnMatch(pattern, newGame.ID)
			if err != nil {
				t.Fatal(err)
			}
			if got := overridesOf(s)[pattern]; got != newGame.ID {
				t.Fatalf("rule after learning = %q, want %q", got, newGame.ID)
			}
			if err := s.UndoMatch(undo); err != nil {
				t.Fatalf("UndoMatch: %v", err)
			}

			for name, view := range map[string]*Service{"memory": s, "disk": mustServiceAt(t, dir)} {
				got := overridesOf(view)
				if len(got) != len(wantRules) || got[pattern] != wantRules[pattern] {
					t.Errorf("%s: rules = %v, want %v", name, got, wantRules)
				}
				if aliases := aliasesOf(t, view, newGame.ID); len(aliases) != 0 {
					t.Errorf("%s: aliases of the new game = %v, want none", name, aliases)
				}
				if aliases := aliasesOf(t, view, oldGame.ID); !slices.Equal(aliases, wantOldAliases) {
					t.Errorf("%s: aliases of the old game = %v, want %v", name, aliases, wantOldAliases)
				}
			}
		})
	}
}

func TestUndoMatchLeavesARuleLearnedMeanwhile(t *testing.T) {
	s := newTestService(t)
	games := seed(t, s, Game{Title: "Cyberpunk 2077"}, Game{Title: "Phantom Liberty"})

	undo, err := s.LearnMatch("cyberpunk 2077 ultimate", games[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.LearnMatch("cyberpunk 2077 ultimate", games[1].ID); err != nil {
		t.Fatal(err)
	}
	if err := s.UndoMatch(undo); err != nil {
		t.Fatalf("UndoMatch: %v", err)
	}

	if got := overridesOf(s)["cyberpunk 2077 ultimate"]; got != games[1].ID {
		t.Fatalf("rule = %q, want the newer rule for %q kept", got, games[1].ID)
	}
}

func TestUndoMatchOfNothingChangesNothing(t *testing.T) {
	s := newTestService(t)
	games := seed(t, s, Game{Title: "Cyberpunk 2077"})
	if _, err := s.LearnMatch("cyberpunk 2077 ultimate", games[0].ID); err != nil {
		t.Fatal(err)
	}

	if err := s.UndoMatch(MatchUndo{}); err != nil {
		t.Fatalf("UndoMatch(zero): %v", err)
	}
	if got := overridesOf(s)["cyberpunk 2077 ultimate"]; got != games[0].ID {
		t.Fatalf("rule = %q, want it kept", got)
	}
}

func TestUndoMatchReportsWhatItCouldNotWrite(t *testing.T) {
	tests := []struct {
		name  string
		block string
	}{
		{"catalog file", "catalog.json"},
		{"overrides file", "match_overrides.json"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			s := mustServiceAt(t, dir)
			games := seed(t, s, Game{Title: "Cyberpunk 2077"})
			undo, err := s.LearnMatch("cyberpunk 2077 ultimate", games[0].ID)
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(dir, tt.block)
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(path, 0o755); err != nil {
				t.Fatal(err)
			}

			if err := s.UndoMatch(undo); err == nil {
				t.Fatal("UndoMatch() error = nil, want the write failure")
			}

			if tt.block == "catalog.json" {
				if got := overridesOf(s)["cyberpunk 2077 ultimate"]; got != games[0].ID {
					t.Fatalf("rule = %q: the undo went on after the alias could not be removed", got)
				}
				if aliases := aliasesOf(t, s, games[0].ID); !slices.Equal(aliases, []string{"cyberpunk 2077 ultimate"}) {
					t.Fatalf("aliases = %v, want memory to match the file that was not changed", aliases)
				}
				return
			}
			if got := overridesOf(s)["cyberpunk 2077 ultimate"]; got != games[0].ID {
				t.Fatalf("rule = %q, want memory to match the file that was not changed", got)
			}
		})
	}
}

package catalog

import "testing"

func TestRepackOfABaseGameDoesNotLandOnAnEpisode(t *testing.T) {
	s := newTestService(t)
	games := seed(t, s,
		Game{Title: "Half-Life 2", GameType: "Main Game"},
		Game{Title: "Half-Life 2: Episode One", GameType: "Main Game"},
		Game{Title: "Half-Life 2: Episode Two", GameType: "Main Game"},
	)

	cases := []struct {
		raw  string
		want Game
	}{
		{"Half-Life 2 [FitGirl Repack] (v1.0 + all DLC)", games[0]},
		{"Half.Life.2.Deluxe.Edition.v1.0.MULTi9", games[0]},
		{"Half-Life 2: Episode One [FitGirl Repack]", games[1]},
		{"Half-Life 2: Episode Two v1.0.4 [DODI Repack]", games[2]},
	}

	for _, tc := range cases {
		t.Run(tc.raw, func(t *testing.T) {
			match := s.Resolve(query(tc.raw))
			if match.Status != StatusMatched {
				t.Fatalf("status = %s, want matched: %+v", match.Status, match)
			}
			if match.GameID != tc.want.ID {
				t.Fatalf("matched %q, want %q", s.TitleOf(match.GameID), tc.want.Title)
			}
		})
	}
}

// Тёзки разных лет — единственный случай, где год решает всё. Без года
// однозначного ответа нет, и матч обязан уйти на ручную проверку, а не выбрать
// ту запись, что оказалась первой в индексе.
func TestNamesakesAreSplitByYear(t *testing.T) {
	s := newTestService(t)
	games := seed(t, s,
		Game{Title: "Resident Evil 4", ReleaseYear: year(2005), GameType: "Main Game"},
		Game{Title: "Resident Evil 4", ReleaseYear: year(2023), GameType: "Remake"},
	)

	t.Run("year picks the right one", func(t *testing.T) {
		for _, tc := range []struct {
			raw  string
			want Game
		}{
			{"Resident Evil 4 (2005) [FitGirl Repack]", games[0]},
			{"Resident Evil 4 (2023) [FitGirl Repack]", games[1]},
		} {
			match := s.Resolve(query(tc.raw))
			if match.Status != StatusMatched {
				t.Errorf("%s: status = %s, want matched", tc.raw, match.Status)
				continue
			}
			if match.GameID != tc.want.ID {
				got := 0
				if g, err := s.GetGame(match.GameID); err == nil && g.ReleaseYear != nil {
					got = *g.ReleaseYear
				}
				t.Errorf("%s: matched the %d edition, want %d", tc.raw, got, *tc.want.ReleaseYear)
			}
		}
	})

	t.Run("no year leaves it to the user", func(t *testing.T) {
		match := s.Resolve(query("Resident Evil 4 [FitGirl Repack]"))
		if match.Status == StatusMatched {
			t.Fatalf("an ambiguous title was matched silently: %+v", match)
		}
		if len(match.Candidates) < 2 {
			t.Fatalf("candidates = %+v, want both editions offered", match.Candidates)
		}
	})
}

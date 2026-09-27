package catalog

import (
	"testing"
)

func TestResolveIgnoresAddonsButKeepsThemInTheCatalog(t *testing.T) {
	s := newTestService(t)
	game := seed(t, s, Game{Title: "Elden Ring", GameType: "Main Game"})[0]
	dlc := seed(t, s, Game{Title: "Elden Ring Shadow of the Erdtree", GameType: "DLC"})[0]

	match := s.Resolve(Query{Title: "Elden Ring Shadow of the Erdtree"})
	if match.Status == StatusMatched && match.GameID == dlc.ID {
		t.Fatalf("a DLC was matched automatically: %+v", match)
	}
	for _, c := range match.Candidates {
		if c.GameID == dlc.ID {
			t.Fatalf("a DLC turned up among the candidates: %+v", match.Candidates)
		}
	}

	if got, err := s.GetGame(dlc.ID); err != nil || got.Title != dlc.Title {
		t.Fatalf("GetGame(dlc) = %+v, %v; the catalog must still hold it", got, err)
	}
	found := false
	for _, g := range s.SearchGames("Shadow of the Erdtree", 10) {
		if g.ID == dlc.ID {
			found = true
		}
	}
	if !found {
		t.Error("searching the catalog no longer finds the DLC")
	}

	base := s.Resolve(Query{Title: "Elden Ring"})
	if base.Status != StatusMatched || base.GameID != game.ID {
		t.Fatalf("the base game stopped matching: %+v", base)
	}
}

func TestResolveMatchesRowsWithoutAType(t *testing.T) {
	s := newTestService(t)
	game := seed(t, s, Game{Title: "Untyped Legacy Row"})[0]

	match := s.Resolve(Query{Title: "Untyped Legacy Row"})
	if match.Status != StatusMatched || match.GameID != game.ID {
		t.Fatalf("a row the backend has not refilled yet stopped matching: %+v", match)
	}
}

func TestExternalIDMatchSkipsAddons(t *testing.T) {
	s := newTestService(t)
	seed(t, s, Game{Title: "Some Add-on", GameType: "DLC", ExternalIDs: ExternalIDs{IGDB: "555"}})

	match := s.Resolve(Query{Title: "Some Add-on", ExternalIDs: ExternalIDs{IGDB: "555"}})
	if match.Status == StatusMatched {
		t.Fatalf("a DLC matched through its external id: %+v", match)
	}
}

// Ручной выбор — решение пользователя, и тип каталожной записи его не
// отменяет: он мог осознанно привязать репак к дополнению.
func TestManualOverrideStillReachesAnAddon(t *testing.T) {
	s := newTestService(t)
	dlc := seed(t, s, Game{Title: "Some Add-on", GameType: "DLC"})[0]

	if err := s.LearnMatch("some add on", dlc.ID); err != nil {
		t.Fatalf("LearnMatch() error = %v", err)
	}
	match := s.Resolve(Query{Title: "Some Add-on"})
	if match.Status != StatusMatched || match.GameID != dlc.ID {
		t.Fatalf("the manual override did not survive the type filter: %+v", match)
	}
}

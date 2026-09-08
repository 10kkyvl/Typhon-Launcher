package catalog

import (
	"fmt"
	"testing"
)

// TestAddGameSurvivesSliceReallocation защищает от ловушки, описанной в
// аудите: если index хранит данные игры через ссылку на бэкинг-массив
// s.games (указатель в элемент или закешированный целиком слайс), append к
// s.games может перевыделить массив, и index молча останется смотреть на
// СТАРЫЙ массив — GetGame/SearchGames для только что добавленной или для
// ранее добавленных игр начнут отдавать не то, что реально лежит в s.games.
//
// AddGame растит s.games с нуля, поэтому почти каждый вызов в начале цикла
// перевыделяет бэкинг-массив (0->1->2->4->8...) — это гарантированно
// проверяет сценарий, а не полагается на удачное совпадение ёмкости.
func TestAddGameSurvivesSliceReallocation(t *testing.T) {
	s := newTestService(t)

	const n = 40
	ids := make([]string, 0, n)
	titles := make([]string, 0, n)
	reallocated := false

	for i := range n {
		prevCap := cap(s.games)
		title := fmt.Sprintf("Game Title %02d", i)
		alias := fmt.Sprintf("alias-%02d", i)

		added, err := s.AddGame(Game{Title: title, Aliases: []string{alias}})
		if err != nil {
			t.Fatalf("AddGame(%d): %v", i, err)
		}
		if cap(s.games) != prevCap {
			reallocated = true
		}

		// Игра, только что добавленная через инкрементальный addToIndexLocked,
		// обязана немедленно и полностью находиться через индекс — даже если
		// именно этот append перевыделил s.games.
		got, err := s.GetGame(added.ID)
		if err != nil {
			t.Fatalf("GetGame(%d) right after AddGame: %v", i, err)
		}
		if got.Title != title {
			t.Fatalf("GetGame(%d).Title = %q, want %q (index entry stale right after add)", i, got.Title, title)
		}

		ids = append(ids, added.ID)
		titles = append(titles, title)
	}

	if !reallocated {
		t.Fatal("test setup never observed a s.games reallocation; strengthen it")
	}

	// Более ранние записи обязаны продолжать отражать s.games ПОСЛЕ того, как
	// более поздние AddGame успели перевыделить бэкинг-массив несколько раз.
	for i, id := range ids {
		want := s.games[i]
		got, err := s.GetGame(id)
		if err != nil {
			t.Fatalf("GetGame(%s) after later reallocations: %v", id, err)
		}
		if got.Title != want.Title || len(got.Aliases) != len(want.Aliases) {
			t.Fatalf("GetGame(%s) = %+v, want %+v (index entry not tracking current s.games)", id, got, want)
		}

		found := s.SearchGames(titles[i], 1)
		if len(found) == 0 || found[0].ID != id {
			t.Fatalf("SearchGames(%q) did not find game %s added earlier, after s.games reallocated", titles[i], id)
		}
	}
}

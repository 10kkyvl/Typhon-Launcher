package catalog

import (
	"strconv"
	"testing"
	"time"
)

// compatService заводит каталог с играми и возвращает их идентификаторы в том
// порядке, в каком они были заданы: AddGame выдаёт идентификатор сам.
// compatService заводит каталог с играми и возвращает их идентификаторы в том
// порядке, в каком они были заданы: AddGame выдаёт идентификатор сам. Каждой
// игре проставляется идентификатор IGDB — общая статистика ключуется им.
func compatService(t *testing.T, titles ...string) (*Service, []string) {
	t.Helper()
	s := newTestService(t)
	ids := make([]string, 0, len(titles))
	for i, title := range titles {
		added, err := s.AddGame(Game{Title: title, ExternalIDs: ExternalIDs{IGDB: igdbFor(i)}})
		if err != nil {
			t.Fatalf("AddGame %q: %v", title, err)
		}
		ids = append(ids, added.ID)
	}
	return s, ids
}

func igdbFor(i int) string { return strconv.Itoa(1000 + i) }

// Игра, про которую никто не отчитывался, в «поедет» не попадает: молчание — не
// подтверждение работоспособности, и обещать человеку запуск на этом основании
// значит соврать.
func TestCompatFilterExcludesGamesWithoutObservations(t *testing.T) {
	s, ids := compatService(t, "Known", "Silent")
	_ = ids
	s.SetCompatLookup(func(id string) (int, int, bool) {
		if id == igdbFor(0) {
			return 9, 10, true
		}
		return 0, 0, false
	})

	page := s.QueryGames(GameQuery{Compat: CompatOnlyWorking})
	if len(page.Items) != 1 || page.Items[0].ID != ids[0] {
		t.Fatalf("страница = %+v", page.Items)
	}
	if page.Total != 1 {
		t.Fatalf("Total = %d, want 1", page.Total)
	}
}

func TestCompatFilterDropsGamesThatMostlyFail(t *testing.T) {
	s, _ := compatService(t, "Bad")
	s.SetCompatLookup(func(string) (int, int, bool) { return 2, 10, true })

	if page := s.QueryGames(GameQuery{Compat: CompatOnlyWorking}); len(page.Items) != 0 {
		t.Fatalf("игра, которая у большинства не идёт, прошла фильтр: %+v", page.Items)
	}
}

// Фильтр обязан применяться до нарезки на страницы, иначе страницы приезжают
// разной длины, а Total врёт.
func TestCompatFilterAppliesBeforePaging(t *testing.T) {
	titles := make([]string, 0, 10)
	for i := range 10 {
		titles = append(titles, string(rune('A'+i)))
	}
	s, _ := compatService(t, titles...)
	working := map[string]bool{igdbFor(0): true, igdbFor(1): true, igdbFor(2): true}
	s.SetCompatLookup(func(id string) (int, int, bool) {
		if working[id] {
			return 5, 5, true
		}
		return 0, 0, false
	})

	page := s.QueryGames(GameQuery{Compat: CompatOnlyWorking, PageSize: 2, Page: 1})
	if page.Total != 3 {
		t.Fatalf("Total = %d, want 3", page.Total)
	}
	if len(page.Items) != 2 {
		t.Fatalf("на странице %d игр, want 2", len(page.Items))
	}
}

// Без общей статистики каталог работает как раньше и ничего не фильтрует молча.
func TestQueryWithoutCompatLookupReturnsEverything(t *testing.T) {
	s, _ := compatService(t, "A", "B")

	if page := s.QueryGames(GameQuery{}); len(page.Items) != 2 || page.Compat != nil {
		t.Fatalf("страница = %d игр, compat = %+v", len(page.Items), page.Compat)
	}
}

func TestQueryCarriesCompatNumbersForTheVisiblePage(t *testing.T) {
	s, ids := compatService(t, "A", "B")
	s.SetCompatLookup(func(id string) (int, int, bool) {
		if id == igdbFor(0) {
			return 8, 10, true
		}
		return 0, 0, false
	})

	page := s.QueryGames(GameQuery{})
	if got, ok := page.Compat[ids[0]]; !ok || got.Works != 8 || got.Total != 10 {
		t.Fatalf("compat[A] = %+v, ok = %v", got, ok)
	}
	if _, ok := page.Compat[ids[1]]; ok {
		t.Fatal("игра без наблюдений получила цифру")
	}
}

// Колбэк зовётся под мьютексом каталога. Если он обратится к каталогу обратно —
// а это первое, что приходит в голову, когда нужен идентификатор IGDB, — запрос
// повиснет намертво. Проверяется именно это: каталог обязан отдать колбэку уже
// переведённый идентификатор и не ждать от него ответных вызовов.
func TestCompatLookupIsNotAskedToCallTheCatalogBack(t *testing.T) {
	s, ids := compatService(t, "A")
	var got string
	s.SetCompatLookup(func(id string) (int, int, bool) {
		got = id
		return 5, 5, true
	})

	done := make(chan struct{})
	go func() {
		defer close(done)
		s.QueryGames(GameQuery{})
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("QueryGames не вернулся: каталог ждёт сам себя")
	}

	if got == ids[0] {
		t.Fatalf("колбэк получил каталожный идентификатор %q — за переводом ему придётся идти обратно в каталог", got)
	}
	if got != igdbFor(0) {
		t.Fatalf("колбэк получил %q, want идентификатор IGDB %q", got, igdbFor(0))
	}
}

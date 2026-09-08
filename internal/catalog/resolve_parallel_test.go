package catalog

import (
	"fmt"
	"sync"
	"testing"
)

// Порядок результатов задаётся индексом запроса, а не тем, какой воркер
// закончил первым. Прогоняем пачку, заведомо большую parallelResolveFloor,
// иначе сработал бы последовательный путь и тест ничего бы не проверял.
func TestResolveAllIsDeterministicAndMatchesTheSerialPath(t *testing.T) {
	s := benchCatalog(t, 2000)
	queries := benchQueries(parallelResolveFloor * 4)

	serial := make([]Match, len(queries))
	for i, q := range queries {
		serial[i] = s.Resolve(q)
	}

	first := s.ResolveAll(queries)
	if len(first) != len(queries) {
		t.Fatalf("ResolveAll returned %d matches, want %d", len(first), len(queries))
	}
	for i := range queries {
		if first[i].Status != serial[i].Status || first[i].GameID != serial[i].GameID {
			t.Fatalf("query %d: parallel = %+v, serial = %+v", i, first[i], serial[i])
		}
	}

	for run := range 5 {
		again := s.ResolveAll(queries)
		for i := range queries {
			if again[i].GameID != first[i].GameID || again[i].Status != first[i].Status {
				t.Fatalf("run %d, query %d: %+v, want %+v", run, i, again[i], first[i])
			}
		}
	}
}

func TestResolveAllHandlesSmallBatches(t *testing.T) {
	s := newTestService(t)
	seed(t, s, Game{Title: "Only Game", GameType: "Main Game"})

	if got := s.ResolveAll(nil); len(got) != 0 {
		t.Fatalf("ResolveAll(nil) = %+v, want empty", got)
	}
	got := s.ResolveAll([]Query{{Title: "Only Game"}, {Title: "Nothing Like It At All"}})
	if len(got) != 2 {
		t.Fatalf("ResolveAll returned %d matches, want 2", len(got))
	}
	if got[0].Status != StatusMatched {
		t.Errorf("first = %+v, want matched", got[0])
	}
	if got[1].Status == StatusMatched {
		t.Errorf("second = %+v, want no match", got[1])
	}
}

// Рефетч источника идёт в фоне и пересекается с чтением из UI и с записью в
// каталог. Тест обязан падать под -race, если резолв возьмёт индекс без лока.
func TestResolveAllRacesWithCatalogWrites(t *testing.T) {
	s := benchCatalog(t, 500)
	queries := benchQueries(parallelResolveFloor * 2)

	var wg sync.WaitGroup
	stop := make(chan struct{})

	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; ; i++ {
			select {
			case <-stop:
				return
			default:
			}
			if _, err := s.AddGame(Game{Title: fmt.Sprintf("Concurrent Addition %d", i), GameType: "Main Game"}); err != nil {
				t.Errorf("AddGame() error = %v", err)
				return
			}
		}
	}()

	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 5 {
				s.ResolveAll(queries)
				s.ListGames()
				s.SearchGames("shadow", 10)
				s.Epoch()
			}
		}()
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		for range 20 {
			if _, err := s.Provision([]Query{{Title: "Provisioned While Resolving"}}); err != nil {
				t.Errorf("Provision() error = %v", err)
				return
			}
		}
	}()

	done := make(chan struct{})
	go func() {
		defer close(done)
		for range 4 {
			s.ResolveAll(queries)
		}
	}()
	<-done
	close(stop)
	wg.Wait()
}

package catalog

import (
	"fmt"
	"testing"

	"typhon/internal/titles"
)

// Данные для замера обязаны быть одинаковыми от прогона к прогону, иначе
// цифры не с чем сравнивать. Целочисленный хэш вместо math/rand: он
// детерминирован по построению и обходится без преобразований между int и
// uint64, на которых gosec справедливо ругается в обе стороны.
func benchPick(seed, step, n int) int {
	if n <= 0 {
		return 0
	}
	h := seed*1000003 ^ step*2654435761
	if h < 0 {
		h = -(h + 1)
	}
	return h % n
}

var benchWords = []string{
	"shadow", "kingdom", "eternal", "war", "chronicles", "legend", "dark", "rise",
	"fallen", "empire", "quest", "storm", "blade", "hunter", "frontier", "echo",
	"crimson", "silent", "iron", "last", "city", "world", "star", "night",
}

func benchTitle(seed, i int) string {
	n := 2 + benchPick(seed, i, 3)
	out := ""
	for w := range n {
		if out != "" {
			out += " "
		}
		out += benchWords[benchPick(seed+w+1, i, len(benchWords))]
	}
	return fmt.Sprintf("%s %d", out, i%900+100)
}

func benchCatalog(tb testing.TB, size int) *Service {
	tb.Helper()
	s, err := NewServiceAt(tb.TempDir())
	if err != nil {
		tb.Fatalf("new catalog: %v", err)
	}
	games := make([]Game, 0, size)
	for i := range size {
		title := benchTitle(1, i)
		games = append(games, Game{
			ID:        fmt.Sprintf("g%06d", i),
			Title:     title,
			SortTitle: sortTitle(title),
			GameType:  "Main Game",
		})
	}
	s.games = games
	s.rebuildLocked()
	return s
}

func benchQueries(count int) []Query {
	out := make([]Query, 0, count)
	for i := range count {
		raw := benchTitle(2, i) + " [FitGirl Repack] v1.2"
		parsed := titles.Parse(raw)
		out = append(out, Query{Title: parsed.Base, Normalized: parsed.Normalized, Year: parsed.Year})
	}
	return out
}

// BenchmarkResolveAll меряет то, что делает рефетч источника: пачку названий
// из фида против всего каталога. Размеры взяты с запасом относительно живых
// данных (у пользователя 27 тысяч релизов на два источника).
func BenchmarkResolveAll(b *testing.B) {
	cases := []struct {
		games   int
		queries int
	}{
		{5000, 1000},
		{50000, 1000},
		{50000, 20000},
	}

	for _, tc := range cases {
		b.Run(fmt.Sprintf("catalog%d_queries%d", tc.games, tc.queries), func(b *testing.B) {
			s := benchCatalog(b, tc.games)
			queries := benchQueries(tc.queries)
			b.ResetTimer()
			for range b.N {
				if got := len(s.ResolveAll(queries)); got != len(queries) {
					b.Fatalf("ResolveAll returned %d matches, want %d", got, len(queries))
				}
			}
		})
	}
}

package catalog

import (
	"fmt"
	"runtime"
	"testing"
	"unsafe"
)

// TestEntryDoesNotEmbedGameByValue отлавливает регресс из аудита памяти:
// entry держал Game целиком по значению, то есть index дублировал заголовки
// всех игр каталога второй раз. Пороговое условие — entry обязан быть заметно
// меньше Game, а не просто "не равен ей".
func TestEntryDoesNotEmbedGameByValue(t *testing.T) {
	gameSize := unsafe.Sizeof(Game{})
	entrySize := unsafe.Sizeof(entry{})
	if entrySize >= gameSize {
		t.Fatalf("unsafe.Sizeof(entry{}) = %d, unsafe.Sizeof(Game{}) = %d: entry still embeds a full Game copy", entrySize, gameSize)
	}
}

// TestEntriesArrayHeapGrowthOnLargeCatalog меряет реальный рост кучи ИМЕННО
// от массива entries на масштабе, близком к каталогу пользователя (23 601
// игра) — изолированно от карт индекса (byToken/byTitle/...) и от временных
// строк titles.Normalize/TokenSet, которые вносят собственный, независимый
// от этой находки шум и делают сравнение по всему buildIndex ненадёжным.
// Дублирование Game стоило бы дополнительно n*sizeof(Game) байт только на
// этот массив; без дублирования рост должен быть в разы меньше.
func TestEntriesArrayHeapGrowthOnLargeCatalog(t *testing.T) {
	const n = 23600

	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)

	entries := make([]entry, n)

	runtime.GC()
	runtime.ReadMemStats(&after)

	grew := after.HeapAlloc - before.HeapAlloc
	// Дублирование Game стоило бы минимум n*sizeof(Game) лишних байт на этот
	// массив. Разрешаем половину этой стоимости как запас на шум round-up'ов
	// аллокатора, но регресс всё ещё обязан быть пойман.
	duplicateCost := uint64(n) * uint64(unsafe.Sizeof(Game{}))
	maxAllowed := duplicateCost / 2
	t.Logf("make([]entry, %d) heap growth: %d bytes (%.1f bytes/entry); sizeof(entry)=%d, sizeof(Game)=%d, one duplicated Game copy per entry would cost >= %d bytes",
		n, grew, float64(grew)/float64(n), unsafe.Sizeof(entry{}), unsafe.Sizeof(Game{}), duplicateCost)
	if grew > maxAllowed {
		t.Fatalf("entries array grew heap by %d bytes over %d games, want <= %d bytes (entry is duplicating Game headers)", grew, n, maxAllowed)
	}

	runtime.KeepAlive(entries)
}

// TestBuildIndexHeapGrowthOnLargeCatalog — то же самое сквозь настоящий
// buildIndex на реалистичных данных, для отчёта: показывает совокупный рост
// (entries + карты индекса + нормализованные строки), не только вклад
// entries. Порог здесь широкий и служит дымовым тестом на взрыв памяти, а не
// точным замером именно этой находки — точный замер даёт тест выше.
func TestBuildIndexHeapGrowthOnLargeCatalog(t *testing.T) {
	const n = 23600
	games := make([]Game, n)
	for i := range games {
		games[i] = Game{
			ID:        fmt.Sprintf("g%06d", i),
			Title:     fmt.Sprintf("Game Title Number %06d", i),
			SortTitle: fmt.Sprintf("game title number %06d", i),
			GameType:  "Main Game",
			Developer: "Some Studio",
			Publisher: "Some Publisher",
			Genres:    []string{"Action", "Adventure"},
		}
	}

	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)

	idx := buildIndex(games)

	runtime.GC()
	runtime.ReadMemStats(&after)

	grew := after.HeapAlloc - before.HeapAlloc
	t.Logf("buildIndex heap growth for %d games (entries + index maps + normalized strings): %d bytes (%.1f bytes/game)", n, grew, float64(grew)/float64(n))

	runtime.KeepAlive(idx)
}

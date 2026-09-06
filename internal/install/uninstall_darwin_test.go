//go:build darwin && !devmock

package install

import "testing"

// Без CrossOver бутылей нет, значит и записей нет — но это не ошибка:
// вызывающий сравнивает снимки «до» и «после», и пустая карта корректна.
func TestReadUninstallEntriesAlwaysReturnsAMap(t *testing.T) {
	got, err := readUninstallEntries()
	if err != nil {
		t.Fatalf("readUninstallEntries: %v", err)
	}
	if got == nil {
		t.Fatal("got nil map, want an empty one")
	}
}

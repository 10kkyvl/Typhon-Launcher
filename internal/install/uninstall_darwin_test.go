//go:build darwin && !devmock

package install

import (
	"os"
	"path/filepath"
	"testing"

	"typhon/internal/wine"
)

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

// TestCollectUninstallEntriesFailsOnBottleReadError закрывает КРИТ-находку:
// раньше ошибка чтения реестра одной бутыли глоталась через continue, и
// функция всё равно отдавала nil-ошибку с тем, что успела собрать по
// остальным бутылям. setRemoval сравнивает такой «пустой, но успешный»
// снимок со снимком «после» и может принять чужую, уже существовавшую
// запись реестра за новую, появившуюся при этой установке — и вернуть её
// деинсталлятор. Ошибка одной бутыли обязана останавливать всё чтение.
func TestCollectUninstallEntriesFailsOnBottleReadError(t *testing.T) {
	goodPath := t.TempDir()
	badPath := t.TempDir()
	// system.reg — каталог, а не файл: os.ReadFile внутри UninstallEntries
	// гарантированно и детерминированно вернёт ошибку, без зависимости от
	// прав доступа или запуска от root.
	if err := os.Mkdir(filepath.Join(badPath, "system.reg"), 0o755); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}

	bottles := []wine.Bottle{
		{Name: "good", Path: goodPath},
		{Name: "bad", Path: badPath},
	}

	got, err := collectUninstallEntries(bottles)
	if err == nil {
		t.Fatalf("collectUninstallEntries = %v, nil — want an error: one bottle's registry could not be read, the snapshot is incomplete and must not look like a clean empty result", got)
	}
}

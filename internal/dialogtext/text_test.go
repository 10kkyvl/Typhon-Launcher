package dialogtext

import "testing"

func TestResolvedUILanguage(t *testing.T) {
	if got := For("ru"); got.AllFiles != "Все файлы" || got.FeedTitle != "Выберите файл источника" {
		t.Fatalf("Russian UI labels: %+v", got)
	}
	english := For("en")
	if english.AllFiles != "All files" || english.TorrentTitle != "Choose a torrent file" {
		t.Fatalf("English UI labels: %+v", english)
	}
	if got := For(""); got != english {
		t.Fatalf("unknown locale must use the same fallback as the frontend: %+v", got)
	}
}

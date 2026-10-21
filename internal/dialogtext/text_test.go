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

func TestChatImageLabelsExistInBothLanguages(t *testing.T) {
	for _, language := range []string{"ru", "en"} {
		got := For(language)
		if got.ChatImageTitle == "" || got.ChatImages == "" {
			t.Fatalf("%s chat image labels are empty: %+v", language, got)
		}
	}
	if For("ru").ChatImages == For("en").ChatImages {
		t.Fatal("chat image filter label is not localised")
	}
}

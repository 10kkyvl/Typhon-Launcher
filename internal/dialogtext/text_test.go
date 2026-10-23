package dialogtext

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

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

func TestTrayAndJumpListLabelsExistInBothLanguages(t *testing.T) {
	for _, language := range []string{"ru", "en"} {
		got := For(language)
		fields := map[string]string{
			"OpenTyphon": got.OpenTyphon, "Quit": got.Quit,
			"RecentGames": got.RecentGames, "Playing": got.Playing, "StopGame": got.StopGame,
			"LibraryTask": got.LibraryTask, "DownloadsTask": got.DownloadsTask,
		}
		for name, value := range fields {
			if value == "" {
				t.Fatalf("%s: %s is empty", language, name)
			}
		}
	}
	ru, en := For("ru"), For("en")
	if ru.RecentGames == en.RecentGames || ru.Playing == en.Playing || ru.StopGame == en.StopGame ||
		ru.LibraryTask == en.LibraryTask || ru.DownloadsTask == en.DownloadsTask {
		t.Fatal("tray and jump list labels are not localised")
	}
}

func TestRollbackLabelsExistInBothLanguagesAndTakeTheirArguments(t *testing.T) {
	for _, language := range []string{"ru", "en"} {
		got := For(language)
		if got.RollbackTitle == "" || got.RollbackAsk == "" || got.RollbackFailed == "" {
			t.Fatalf("%s rollback labels are empty: %+v", language, got)
		}
		if fmt.Sprintf(got.RollbackAsk, "1.2.0") == got.RollbackAsk || !strings.Contains(fmt.Sprintf(got.RollbackAsk, "1.2.0"), "1.2.0") {
			t.Fatalf("%s question does not take the version: %q", language, got.RollbackAsk)
		}
		if !strings.Contains(fmt.Sprintf(got.RollbackFailed, errors.New("boom")), "boom") {
			t.Fatalf("%s failure text does not take the reason: %q", language, got.RollbackFailed)
		}
	}
	if For("ru").RollbackAsk == For("en").RollbackAsk {
		t.Fatal("the rollback question is not localised")
	}
}

package presence

import (
	"errors"
	"strings"
	"testing"
	"time"

	"typhon/internal/discord"
	"typhon/internal/library"
	"typhon/internal/settings"
)

type fakeTarget struct {
	shown   []discord.Presence
	cleared int
	enabled []bool
}

func (f *fakeTarget) Show(p discord.Presence) { f.shown = append(f.shown, p) }
func (f *fakeTarget) Clear()                  { f.cleared++ }
func (f *fakeTarget) SetEnabled(v bool)       { f.enabled = append(f.enabled, v) }

func newWatcher(t *testing.T, cover CoverFunc) (*Watcher, *fakeTarget) {
	t.Helper()
	target := &fakeTarget{}
	w, err := New(target, cover)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	w.now = func() time.Time { return time.Unix(1700000000, 0) }
	return w, target
}

func noCover(string) (string, error) { return "", nil }

func TestNewRejectsMissingDependencies(t *testing.T) {
	if _, err := New(nil, noCover); !errors.Is(err, errNoTarget) {
		t.Fatalf("target nil: %v", err)
	}
	if _, err := New(&fakeTarget{}, nil); !errors.Is(err, errNoCover) {
		t.Fatalf("cover nil: %v", err)
	}
}

func TestSessionStartedCover(t *testing.T) {
	cases := []struct {
		name       string
		canonical  string
		cover      CoverFunc
		largeImage string
		largeText  string
		smallImage string
	}{
		{
			name:       "https обложка становится большой картинкой",
			canonical:  "abc",
			cover:      func(string) (string, error) { return "https://images.igdb.com/co.jpg", nil },
			largeImage: "https://images.igdb.com/co.jpg",
			largeText:  "Cyberpunk 2077",
			smallImage: assetKey,
		},
		{
			name:       "http обложка отвергается",
			canonical:  "abc",
			cover:      func(string) (string, error) { return "http://images.igdb.com/co.jpg", nil },
			largeImage: assetKey,
			largeText:  appName,
		},
		{
			name:       "локальный путь отвергается",
			canonical:  "abc",
			cover:      func(string) (string, error) { return "/media/games/co.jpg", nil },
			largeImage: assetKey,
			largeText:  appName,
		},
		{
			name:       "ошибка резолвера не роняет присутствие",
			canonical:  "abc",
			cover:      func(string) (string, error) { return "", errors.New("нет каталога") },
			largeImage: assetKey,
			largeText:  appName,
		},
		{
			name:      "без canonical id резолвер не зовётся",
			canonical: "",
			cover: func(string) (string, error) {
				t.Fatal("резолвер не должен вызываться")
				return "", nil
			},
			largeImage: assetKey,
			largeText:  appName,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w, target := newWatcher(t, tc.cover)
			w.SessionStarted(library.Game{ID: "1", Title: "Cyberpunk 2077", CanonicalGameID: tc.canonical})
			if len(target.shown) != 1 {
				t.Fatalf("показов: %d", len(target.shown))
			}
			got := target.shown[0]
			if got.LargeImage != tc.largeImage {
				t.Errorf("LargeImage = %q, ждали %q", got.LargeImage, tc.largeImage)
			}
			if got.LargeText != tc.largeText {
				t.Errorf("LargeText = %q, ждали %q", got.LargeText, tc.largeText)
			}
			if got.SmallImage != tc.smallImage {
				t.Errorf("SmallImage = %q, ждали %q", got.SmallImage, tc.smallImage)
			}
			if got.Details != "Cyberpunk 2077" {
				t.Errorf("Details = %q", got.Details)
			}
			if !got.StartedAt.Equal(time.Unix(1700000000, 0)) {
				t.Errorf("StartedAt = %v", got.StartedAt)
			}
		})
	}
}

func TestPlaytimeState(t *testing.T) {
	cases := []struct {
		seconds int64
		want    string
	}{
		{0, ""},
		{59, ""},
		{60, "Всего 1 мин"},
		{3599, "Всего 59 мин"},
		{3600, "Всего 1 ч"},
		{45000, "Всего 12 ч"},
	}
	for _, tc := range cases {
		w, target := newWatcher(t, noCover)
		w.SessionStarted(library.Game{ID: "1", Title: "Игра", PlaytimeSeconds: tc.seconds})
		if got := target.shown[0].State; got != tc.want {
			t.Errorf("%d секунд: State = %q, ждали %q", tc.seconds, got, tc.want)
		}
	}
}

func TestTitleFallbackAndClamp(t *testing.T) {
	w, target := newWatcher(t, noCover)
	w.SessionStarted(library.Game{ID: "1", Title: "  "})
	if got := target.shown[0].Details; got != untitled {
		t.Errorf("пустое название: Details = %q", got)
	}

	long := strings.Repeat("я", maxText+40)
	w, target = newWatcher(t, noCover)
	w.SessionStarted(library.Game{ID: "1", Title: long})
	if got := []rune(target.shown[0].Details); len(got) != maxText {
		t.Errorf("длина Details = %d, ждали %d", len(got), maxText)
	}

	w, target = newWatcher(t, noCover)
	w.SessionStarted(library.Game{ID: "1", Title: "A"})
	if got := target.shown[0].Details; got != untitled {
		t.Errorf("однобуквенное название: Details = %q", got)
	}
}

func TestSessionStoppedClears(t *testing.T) {
	w, target := newWatcher(t, noCover)
	w.SessionStopped("1")
	if target.cleared != 1 {
		t.Fatalf("Clear вызван %d раз", target.cleared)
	}
	if len(target.shown) != 0 {
		t.Fatalf("лишние показы: %d", len(target.shown))
	}
}

func TestApplyPassesToggle(t *testing.T) {
	w, target := newWatcher(t, noCover)
	w.Apply(settings.Settings{DiscordRichPresence: true})
	w.Apply(settings.Settings{DiscordRichPresence: false})
	if len(target.enabled) != 2 || !target.enabled[0] || target.enabled[1] {
		t.Fatalf("SetEnabled: %v", target.enabled)
	}
}

func TestWatcherSatisfiesLibraryHook(t *testing.T) {
	w, _ := newWatcher(t, noCover)
	var _ library.SessionWatcher = w
}

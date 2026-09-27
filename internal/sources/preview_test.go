package sources

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"typhon/internal/catalog"
)

func previewEntries(t *testing.T, titles ...string) []feedEntry {
	t.Helper()
	out := make([]feedEntry, 0, len(titles))
	for i, title := range titles {
		out = append(out, feedEntry{Title: title, URIs: []string{magnetOf(string(rune('a'+i)) + "1")}})
	}
	return out
}

func previewService(t *testing.T, games ...catalog.Game) *Service {
	t.Helper()
	dir := t.TempDir()
	cat := mustCatalog(t, dir)
	for _, g := range games {
		if _, err := cat.AddGame(g); err != nil {
			t.Fatalf("add game %q: %v", g.Title, err)
		}
	}
	return mustServiceAt(t, dir, cat)
}

func TestPreviewCountsGamesAlreadyInTheCatalog(t *testing.T) {
	s := previewService(t,
		catalog.Game{Title: "Elden Ring", GameType: "Main Game"},
		catalog.Game{Title: "Portal 2", GameType: "Main Game"},
	)

	path := feedFile(t, "Preview Source", previewEntries(t,
		"Elden Ring [FitGirl Repack] (v1.12 + all DLC)",
		"Elden Ring Deluxe Edition v1.10",
		"Portal 2 [DODI Repack]",
		"Some Game Nobody Has v1.0",
		"Another Unknown Game v2.0",
	)...)

	preview, err := s.TestSourceFile(path)
	if err != nil {
		t.Fatalf("TestSourceFile() error = %v", err)
	}

	if preview.Entries != 5 {
		t.Errorf("Entries = %d, want 5", preview.Entries)
	}
	// Два репака Elden Ring — одна игра.
	if preview.Games != 4 {
		t.Errorf("Games = %d, want 4 distinct games", preview.Games)
	}
	if preview.Known != 2 {
		t.Errorf("Known = %d, want 2", preview.Known)
	}
	if preview.Unknown != preview.Games-preview.Known {
		t.Errorf("Unknown = %d, want %d", preview.Unknown, preview.Games-preview.Known)
	}
}

func TestPreviewOfAnEmptyCatalogKnowsNothing(t *testing.T) {
	s := previewService(t)
	path := feedFile(t, "Preview Source", previewEntries(t, "Elden Ring v1.0", "Portal 2 v1.0")...)

	preview, err := s.TestSourceFile(path)
	if err != nil {
		t.Fatalf("TestSourceFile() error = %v", err)
	}
	if preview.Known != 0 {
		t.Errorf("Known = %d, want 0", preview.Known)
	}
	if preview.Unknown != 2 {
		t.Errorf("Unknown = %d, want 2", preview.Unknown)
	}
}

// Превью и «Сохранить» — два шага одного мастера. Второй не должен качать те
// же мегабайты снова.
func TestAddSourceReusesThePreviewedFeed(t *testing.T) {
	var mu sync.Mutex
	var hits int
	body := feedBody(t, "Preview Source", previewEntries(t, "Elden Ring v1.0", "Portal 2 v1.0")...)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		hits++
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write([]byte(body)); err != nil {
			t.Errorf("write: %v", err)
		}
	}))
	defer srv.Close()

	s := previewService(t)

	if _, err := s.TestSource(srv.URL); err != nil {
		t.Fatalf("TestSource() error = %v", err)
	}
	mu.Lock()
	afterPreview := hits
	mu.Unlock()
	if afterPreview != 1 {
		t.Fatalf("preview made %d requests, want 1", afterPreview)
	}

	src, err := s.AddSource(srv.URL)
	if err != nil {
		t.Fatalf("AddSource() error = %v", err)
	}
	mu.Lock()
	afterAdd := hits
	mu.Unlock()
	if afterAdd != 1 {
		t.Fatalf("saving the source downloaded the feed again: %d requests total", afterAdd)
	}
	if src.Entries != 2 {
		t.Fatalf("Entries = %d, want 2", src.Entries)
	}
}

// Кэш одноразовый: обновление источника обязано сходить в сеть, иначе оно
// показывало бы содержимое, которое пользователь видел в превью.
func TestRefreshAfterAddGoesBackToTheNetwork(t *testing.T) {
	var mu sync.Mutex
	var hits int
	body := feedBody(t, "Preview Source", previewEntries(t, "Elden Ring v1.0")...)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		hits++
		mu.Unlock()
		if _, err := w.Write([]byte(body)); err != nil {
			t.Errorf("write: %v", err)
		}
	}))
	defer srv.Close()

	s := previewService(t)

	if _, err := s.TestSource(srv.URL); err != nil {
		t.Fatalf("TestSource() error = %v", err)
	}
	src, err := s.AddSource(srv.URL)
	if err != nil {
		t.Fatalf("AddSource() error = %v", err)
	}
	if _, err := s.RefreshSource(src.ID); err != nil {
		t.Fatalf("RefreshSource() error = %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if hits != 2 {
		t.Fatalf("requests = %d, want 2: preview, then a real refresh", hits)
	}
}

func TestPreviewOfADifferentSourceDoesNotServeTheCache(t *testing.T) {
	s := previewService(t)

	first := feedFile(t, "First", previewEntries(t, "Elden Ring v1.0")...)
	second := feedFile(t, "Second", previewEntries(t, "Portal 2 v1.0", "Half-Life 2 v1.0")...)

	if _, err := s.TestSourceFile(first); err != nil {
		t.Fatalf("TestSourceFile(first) error = %v", err)
	}
	src, err := s.AddSourceFile(second)
	if err != nil {
		t.Fatalf("AddSourceFile(second) error = %v", err)
	}
	if src.Entries != 2 {
		t.Fatalf("Entries = %d, want 2: the second source got the first one's cached feed", src.Entries)
	}
}

func TestInsecureSourceIsFlagged(t *testing.T) {
	cases := []struct {
		raw  string
		want bool
	}{
		{"http://example.com/feed.json", true},
		{"HTTP://Example.com/feed.json", true},
		{"https://example.com/feed.json", false},
	}
	for _, tc := range cases {
		t.Run(tc.raw, func(t *testing.T) {
			if got := insecureURL(tc.raw); got != tc.want {
				t.Fatalf("insecureURL(%q) = %v, want %v", tc.raw, got, tc.want)
			}
		})
	}
}

func TestAddedSourceCarriesTheInsecureFlag(t *testing.T) {
	body := feedBody(t, "Preview Source", previewEntries(t, "Elden Ring v1.0")...)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if _, err := w.Write([]byte(body)); err != nil {
			t.Errorf("write: %v", err)
		}
	}))
	defer srv.Close()

	s := previewService(t)
	preview, err := s.TestSource(srv.URL)
	if err != nil {
		t.Fatalf("TestSource() error = %v", err)
	}
	if !preview.Insecure {
		t.Error("Preview.Insecure = false for an http feed")
	}

	src, err := s.AddSource(srv.URL)
	if err != nil {
		t.Fatalf("AddSource() error = %v", err)
	}
	if !src.Insecure {
		t.Error("Source.Insecure = false for an http feed")
	}
}

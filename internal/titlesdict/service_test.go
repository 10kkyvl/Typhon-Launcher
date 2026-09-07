package titlesdict

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"typhon/internal/titles"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// Активный словарь — состояние пакета titles на весь процесс. Каждый тест
// возвращает его на место, иначе следующий проверял бы чужой словарь.
func restoreActive(t *testing.T) {
	t.Helper()
	previous := titles.Active()
	t.Cleanup(func() { titles.SetActive(previous) })
}

func newService(t *testing.T, base string) (*Service, string) {
	t.Helper()
	restoreActive(t)
	dir := t.TempDir()
	s, err := NewServiceAt(dir, base)
	if err != nil {
		t.Fatalf("NewServiceAt() error = %v", err)
	}
	return s, dir
}

func writeLayer(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

func TestWithoutLayersTheBuiltinDictionaryIsActive(t *testing.T) {
	s, _ := newService(t, "")
	status := s.Status()
	if status.RemoteApplied || status.UserApplied {
		t.Fatalf("status = %+v, want no layers", status)
	}
	if status.LastError != "" {
		t.Fatalf("LastError = %q, want empty", status.LastError)
	}
	if titles.Repacker([]string{"fitgirl"}) != "fitgirl" {
		t.Error("the built-in repacker list is not in effect")
	}
}

func TestUserLayerIsApplied(t *testing.T) {
	dir := t.TempDir()
	restoreActive(t)
	writeLayer(t, dir, userFileName, `{"version":1,"releaseTags":{"someguy":"someguy"},"repackerPriority":["someguy"]}`)

	s, err := NewServiceAt(dir, "")
	if err != nil {
		t.Fatalf("NewServiceAt() error = %v", err)
	}
	if !s.Status().UserApplied {
		t.Fatalf("status = %+v, want UserApplied", s.Status())
	}
	if got := titles.Repacker([]string{"someguy"}); got != "someguy" {
		t.Fatalf("Repacker = %q, want someguy", got)
	}
}

func TestUserLayerWinsOverTheRemoteOne(t *testing.T) {
	dir := t.TempDir()
	restoreActive(t)
	writeLayer(t, dir, remoteFileName, `{"version":1,"releaseTags":{"shared":"from-remote"}}`)
	writeLayer(t, dir, userFileName, `{"version":1,"releaseTags":{"shared":"from-user"}}`)

	if _, err := NewServiceAt(dir, ""); err != nil {
		t.Fatalf("NewServiceAt() error = %v", err)
	}

	parsed := titles.Parse("Some Game shared")
	found := false
	for _, tag := range parsed.Tags {
		if tag == "from-user" {
			found = true
		}
		if tag == "from-remote" {
			t.Fatalf("remote layer won over the user one: tags = %v", parsed.Tags)
		}
	}
	if !found {
		t.Fatalf("user layer not applied: tags = %v", parsed.Tags)
	}
}

func TestBrokenLayerIsSkippedAndReported(t *testing.T) {
	cases := []struct {
		name string
		file string
		body string
	}{
		{"broken user json", userFileName, `{"version":1,`},
		{"broken remote json", remoteFileName, `not json`},
		{"unsupported version", userFileName, `{"version":99}`},
		{"blank entry", userFileName, `{"version":1,"langCodes":["  "]}`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			restoreActive(t)
			writeLayer(t, dir, tc.file, tc.body)

			s, err := NewServiceAt(dir, "")
			if err != nil {
				t.Fatalf("NewServiceAt() error = %v, want the service to start on the built-in layer", err)
			}
			status := s.Status()
			if status.LastError == "" {
				t.Fatalf("status = %+v, want the broken layer reported", status)
			}
			if status.UserApplied || status.RemoteApplied {
				t.Fatalf("status = %+v, want the broken layer skipped", status)
			}
			if titles.Repacker([]string{"fitgirl"}) != "fitgirl" {
				t.Error("a broken layer wiped the built-in dictionary")
			}
		})
	}
}

func TestOversizedLayerIsRejected(t *testing.T) {
	dir := t.TempDir()
	restoreActive(t)
	body := `{"version":1,"summaryPadding":"` + strings.Repeat("a", titles.MaxDictBytes) + `"}`
	writeLayer(t, dir, userFileName, body)

	s, err := NewServiceAt(dir, "")
	if err != nil {
		t.Fatalf("NewServiceAt() error = %v", err)
	}
	if s.Status().UserApplied {
		t.Fatal("an oversized layer was applied")
	}
	if s.Status().LastError == "" {
		t.Fatal("an oversized layer was not reported")
	}
}

func TestRefreshStoresAndAppliesTheServerLayer(t *testing.T) {
	const layer = `{"version":1,"releaseTags":{"newguy":"newguy"},"repackerPriority":["newguy"]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != remotePath {
			t.Errorf("path = %q, want %q", r.URL.Path, remotePath)
		}
		w.Header().Set("ETag", `"abc"`)
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write([]byte(layer)); err != nil {
			t.Errorf("write: %v", err)
		}
	}))
	defer srv.Close()

	s, dir := newService(t, srv.URL)
	if err := s.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}

	saved, err := os.ReadFile(filepath.Join(dir, remoteFileName))
	if err != nil {
		t.Fatalf("read cached layer: %v", err)
	}
	if string(saved) != layer {
		t.Fatalf("cached layer = %s", saved)
	}
	if !s.Status().RemoteApplied {
		t.Fatalf("status = %+v, want RemoteApplied", s.Status())
	}
	if got := titles.Repacker([]string{"newguy"}); got != "newguy" {
		t.Fatalf("Repacker = %q, want newguy", got)
	}
	if s.Status().RemoteETag != `"abc"` {
		t.Fatalf("RemoteETag = %q", s.Status().RemoteETag)
	}
}

func TestRefreshSendsTheETagAndSkipsUnchanged(t *testing.T) {
	var mu sync.Mutex
	var conditional int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if r.Header.Get("If-None-Match") == `"abc"` {
			conditional++
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", `"abc"`)
		if _, err := w.Write([]byte(`{"version":1}`)); err != nil {
			t.Errorf("write: %v", err)
		}
	}))
	defer srv.Close()

	s, _ := newService(t, srv.URL)
	if err := s.Refresh(context.Background()); err != nil {
		t.Fatalf("first Refresh() error = %v", err)
	}
	if err := s.Refresh(context.Background()); err != nil {
		t.Fatalf("second Refresh() error = %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if conditional != 1 {
		t.Fatalf("conditional requests = %d, want 1", conditional)
	}
}

func TestRefreshLeavesTheCacheAloneOnBadResponses(t *testing.T) {
	cases := []struct {
		name    string
		handler http.HandlerFunc
	}{
		{"broken json", func(w http.ResponseWriter, _ *http.Request) {
			if _, err := w.Write([]byte(`{"version":1,`)); err != nil {
				return
			}
		}},
		{"unsupported version", func(w http.ResponseWriter, _ *http.Request) {
			if _, err := w.Write([]byte(`{"version":99}`)); err != nil {
				return
			}
		}},
		{"server error", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}},
		{"oversized body", func(w http.ResponseWriter, _ *http.Request) {
			if _, err := w.Write([]byte(`{"version":1,"pad":"` + strings.Repeat("a", titles.MaxDictBytes) + `"}`)); err != nil {
				return
			}
		}},
	}

	const good = `{"version":1,"releaseTags":{"keeper":"keeper"}}`
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(tc.handler)
			defer srv.Close()

			dir := t.TempDir()
			restoreActive(t)
			writeLayer(t, dir, remoteFileName, good)

			s, err := NewServiceAt(dir, srv.URL)
			if err != nil {
				t.Fatalf("NewServiceAt() error = %v", err)
			}
			if err := s.Refresh(context.Background()); err == nil {
				t.Fatal("Refresh() = nil error, want a failure")
			}

			saved, err := os.ReadFile(filepath.Join(dir, remoteFileName))
			if err != nil {
				t.Fatalf("read cached layer: %v", err)
			}
			if string(saved) != good {
				t.Fatalf("a bad response overwrote the cache: %s", saved)
			}
		})
	}
}

func TestRefreshHonoursACancelledContext(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if _, err := w.Write([]byte(`{"version":1}`)); err != nil {
			return
		}
	}))
	defer srv.Close()

	s, _ := newService(t, srv.URL)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := s.Refresh(ctx); err == nil {
		t.Fatal("Refresh() = nil error on a cancelled context")
	}
}

func TestShutdownStopsTheRefreshLoop(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if _, err := w.Write([]byte(`{"version":1}`)); err != nil {
			return
		}
	}))
	defer srv.Close()

	s, _ := newService(t, srv.URL)
	if err := s.ServiceStartup(context.Background(), application.ServiceOptions{}); err != nil {
		t.Fatalf("ServiceStartup() error = %v", err)
	}
	if err := s.ServiceShutdown(); err != nil {
		t.Fatalf("ServiceShutdown() error = %v", err)
	}
}

func TestReloadPicksUpAnEditedUserFile(t *testing.T) {
	s, dir := newService(t, "")
	if s.Status().UserApplied {
		t.Fatal("UserApplied before the file exists")
	}

	writeLayer(t, dir, userFileName, `{"version":1,"releaseTags":{"latecomer":"latecomer"},"repackerPriority":["latecomer"]}`)
	if err := s.Reload(); err != nil {
		t.Fatalf("Reload() error = %v", err)
	}
	if !s.Status().UserApplied {
		t.Fatalf("status = %+v, want UserApplied", s.Status())
	}
	if got := titles.Repacker([]string{"latecomer"}); got != "latecomer" {
		t.Fatalf("Repacker = %q, want latecomer", got)
	}
}

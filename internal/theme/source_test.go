package theme

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
)

type recorded struct {
	name string
	data any
}

type recorder struct {
	mu     sync.Mutex
	events []recorded
	notify chan string
}

func (r *recorder) emit(name string, data any) {
	r.mu.Lock()
	r.events = append(r.events, recorded{name, data})
	r.mu.Unlock()
	if r.notify != nil {
		select {
		case r.notify <- name:
		default:
		}
	}
}

func (r *recorder) named(name string) []recorded {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []recorded
	for _, e := range r.events {
		if e.name == name {
			out = append(out, e)
		}
	}
	return out
}

func (r *recorder) reset() {
	r.mu.Lock()
	r.events = nil
	r.mu.Unlock()
}

type watched struct {
	s    *Service
	path string
	rec  *recorder
}

func newWatched(t *testing.T) watched {
	t.Helper()
	s := mustServiceAt(t, filepath.Join(t.TempDir(), "themes.json"))
	rec := &recorder{}
	s.events = rec.emit
	path := importFilePath(t, "live.typhontheme", []byte(themeText("live", "Исходная")))
	if _, err := s.Import(path); err != nil {
		t.Fatalf("Import() error = %v", err)
	}
	rec.reset()
	return watched{s: s, path: path, rec: rec}
}

func writeAhead(path, body string, ahead time.Duration) error {
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		return err
	}
	later := time.Now().Add(ahead)
	return os.Chtimes(path, later, later)
}

func writeLater(t *testing.T, path, body string, ahead time.Duration) {
	t.Helper()
	if err := writeAhead(path, body, ahead); err != nil {
		t.Fatal(err)
	}
}

func TestPollOnceUnchangedFileEmitsNothing(t *testing.T) {
	w := newWatched(t)
	w.s.pollOnce(t.Context())
	w.s.pollOnce(t.Context())
	if n := len(w.rec.named("theme:updated")) + len(w.rec.named("theme:source-failed")); n != 0 {
		t.Fatalf("poll of an unchanged file emitted %d events", n)
	}
}

func TestPollOnceReloadsChangedFile(t *testing.T) {
	w := newWatched(t)
	w.s.pollOnce(t.Context())
	writeLater(t, w.path, themeText("other-id", "Изменённая"), time.Hour)

	w.s.pollOnce(t.Context())
	got, err := w.s.Get("live")
	if err != nil {
		t.Fatalf("Get(live) error = %v", err)
	}
	if got.Name != "Изменённая" || got.Source != filepath.Clean(w.path) {
		t.Fatalf("after reload = %+v, want new name and same source", got)
	}
	if _, err := w.s.Get("other-id"); err == nil {
		t.Fatal("reload took the id from the file instead of keeping the stored one")
	}
	if n := len(w.rec.named("theme:updated")); n != 1 {
		t.Fatalf("theme:updated emitted %d times, want 1", n)
	}

	w.s.pollOnce(t.Context())
	if n := len(w.rec.named("theme:updated")); n != 1 {
		t.Fatalf("second poll of the same file emitted again: %d", n)
	}
}

func TestPollOnceFirstLookNewerThanTheme(t *testing.T) {
	w := newWatched(t)
	writeLater(t, w.path, themeText("live", "Пока лаунчер спал"), time.Hour)
	w.s.pollOnce(t.Context())
	got, err := w.s.Get("live")
	if err != nil {
		t.Fatalf("Get(live) error = %v", err)
	}
	if got.Name != "Пока лаунчер спал" {
		t.Fatalf("first poll ignored a file newer than the theme: %+v", got)
	}
}

func TestPollOnceFailureKeepsThemeAndReportsOnce(t *testing.T) {
	cases := []struct {
		name      string
		breakFile func(t *testing.T, w watched)
		wantCode  string
	}{
		{
			name: "half-written json",
			breakFile: func(t *testing.T, w watched) {
				writeLater(t, w.path, `{"version":1,"theme":{"id":"live"`, time.Hour)
			},
			wantCode: "theme.invalid_format",
		},
		{
			name: "invalid token",
			breakFile: func(t *testing.T, w watched) {
				writeLater(t, w.path, `{"version":1,"theme":{"id":"live","name":"X","base":"dark","tokens":{"--bg":"url(x)"}}}`, time.Hour)
			},
			wantCode: "theme.invalid_token_value",
		},
		{
			name: "deleted file",
			breakFile: func(t *testing.T, w watched) {
				if err := os.Remove(w.path); err != nil {
					t.Fatal(err)
				}
			},
			wantCode: "",
		},
		{
			name: "file replaced by directory",
			breakFile: func(t *testing.T, w watched) {
				if err := os.Remove(w.path); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(w.path, 0o700); err != nil {
					t.Fatal(err)
				}
				later := time.Now().Add(time.Hour)
				if err := os.Chtimes(w.path, later, later); err != nil {
					t.Fatal(err)
				}
			},
			wantCode: "",
		},
		{
			name: "persist failure",
			breakFile: func(t *testing.T, w watched) {
				blocker := filepath.Join(t.TempDir(), "blocker")
				if err := os.WriteFile(blocker, []byte("not a directory"), 0o600); err != nil {
					t.Fatal(err)
				}
				w.s.path = filepath.Join(blocker, "themes.json")
				writeLater(t, w.path, themeText("live", "Не сохранится"), time.Hour)
			},
			wantCode: "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := newWatched(t)
			w.s.pollOnce(t.Context())
			before, err := w.s.Get("live")
			if err != nil {
				t.Fatalf("Get(live) error = %v", err)
			}

			tc.breakFile(t, w)
			w.s.pollOnce(t.Context())
			w.s.pollOnce(t.Context())

			after, err := w.s.Get("live")
			if err != nil {
				t.Fatalf("Get(live) after failure error = %v", err)
			}
			if after.Name != before.Name || !after.UpdatedAt.Equal(before.UpdatedAt) || after.Source != before.Source {
				t.Fatalf("failed reload changed the theme: before %+v, after %+v", before, after)
			}
			if n := len(w.rec.named("theme:updated")); n != 0 {
				t.Fatalf("failed reload emitted theme:updated %d times", n)
			}
			failures := w.rec.named("theme:source-failed")
			if len(failures) != 1 {
				t.Fatalf("theme:source-failed emitted %d times over two polls, want 1", len(failures))
			}
			f, ok := failures[0].data.(SourceFailure)
			if !ok {
				t.Fatalf("event payload %T, want SourceFailure", failures[0].data)
			}
			if f.ID != "live" || f.Name != before.Name || f.Code != tc.wantCode || f.Message == "" {
				t.Fatalf("SourceFailure = %+v, want id live, name %q, code %q", f, before.Name, tc.wantCode)
			}
		})
	}
}

func TestPollOnceRecoversAfterFailure(t *testing.T) {
	w := newWatched(t)
	w.s.pollOnce(t.Context())
	writeLater(t, w.path, `{"version":`, time.Hour)
	w.s.pollOnce(t.Context())
	writeLater(t, w.path, themeText("live", "Дописана"), 2*time.Hour)
	w.s.pollOnce(t.Context())
	got, err := w.s.Get("live")
	if err != nil {
		t.Fatalf("Get(live) error = %v", err)
	}
	if got.Name != "Дописана" {
		t.Fatalf("finished file not reloaded: %+v", got)
	}
}

func TestPollOnceIgnoresDetachedTheme(t *testing.T) {
	w := newWatched(t)
	w.s.pollOnce(t.Context())
	stored, err := w.s.Get("live")
	if err != nil {
		t.Fatal(err)
	}
	stored.Name = "Правка в лаунчере"
	if _, err := w.s.Save(stored); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	writeLater(t, w.path, themeText("live", "Из файла"), time.Hour)
	w.s.pollOnce(t.Context())
	got, err := w.s.Get("live")
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "Правка в лаунчере" {
		t.Fatalf("saved theme was overwritten from its old file: %+v", got)
	}
}

func TestPollOnceStopsOnCancelledContext(t *testing.T) {
	w := newWatched(t)
	writeLater(t, w.path, themeText("live", "Не применится"), time.Hour)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	w.s.pollOnce(ctx)
	got, err := w.s.Get("live")
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "Исходная" {
		t.Fatalf("cancelled poll reloaded the theme: %+v", got)
	}
}

func TestWatcherReloadsAndStopsOnCancel(t *testing.T) {
	w := newWatched(t)
	w.rec.notify = make(chan string, 16)
	w.s.pollEvery = time.Millisecond
	writeLater(t, w.path, themeText("live", "Из горутины"), time.Hour)

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	if err := w.s.ServiceStartup(ctx, application.ServiceOptions{}); err != nil {
		t.Fatalf("ServiceStartup() error = %v", err)
	}
	if err := w.s.ServiceStartup(ctx, application.ServiceOptions{}); err == nil {
		t.Fatal("second ServiceStartup() succeeded, want error")
	}

	deadline := time.After(10 * time.Second)
	for updated := false; !updated; {
		select {
		case name := <-w.rec.notify:
			updated = name == "theme:updated"
		case <-deadline:
			t.Fatal("watcher did not reload the changed file")
		}
	}

	cancel()
	done := make(chan struct{})
	go func() {
		w.s.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("watcher goroutine still running after ctx cancel")
	}
	if err := w.s.ServiceShutdown(); err != nil {
		t.Fatalf("ServiceShutdown() error = %v", err)
	}
}

func TestPollOnceConcurrentWithSave(t *testing.T) {
	w := newWatched(t)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := range 40 {
			if err := writeAhead(w.path, themeText("live", "Версия "+string(rune('A'+i))), time.Duration(i+1)*time.Minute); err != nil {
				t.Error(err)
				return
			}
			w.s.pollOnce(t.Context())
		}
	}()
	go func() {
		defer wg.Done()
		for range 40 {
			if _, err := w.s.Save(validTheme("other")); err != nil {
				t.Errorf("Save() error = %v", err)
				return
			}
			if len(w.s.List()) == 0 {
				t.Error("List() returned no themes")
				return
			}
		}
	}()
	wg.Wait()
	got, err := w.s.Get("live")
	if err != nil {
		t.Fatal(err)
	}
	if got.Source != filepath.Clean(w.path) {
		t.Fatalf("source lost under concurrency: %+v", got)
	}
}

func TestPollOnceMissingAtStartIsNotReported(t *testing.T) {
	w := newWatched(t)
	if err := os.Remove(w.path); err != nil {
		t.Fatal(err)
	}
	w.s.pollOnce(t.Context())
	w.s.pollOnce(t.Context())
	if n := len(w.rec.named("theme:source-failed")); n != 0 {
		t.Fatalf("a file already missing at the first look was reported %d times", n)
	}
	writeLater(t, w.path, themeText("live", "Вернулась"), time.Hour)
	w.s.pollOnce(t.Context())
	got, err := w.s.Get("live")
	if err != nil {
		t.Fatalf("Get(live) error = %v", err)
	}
	if got.Name != "Вернулась" {
		t.Fatalf("file that came back was not reloaded: %+v", got)
	}
}

func TestPollOnceRetriesAPersistFailureWithoutAFileChange(t *testing.T) {
	w := newWatched(t)
	w.s.pollOnce(t.Context())
	good := w.s.path
	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	w.s.path = filepath.Join(blocker, "themes.json")
	writeLater(t, w.path, themeText("live", "После сбоя"), time.Hour)
	w.s.pollOnce(t.Context())
	w.s.pollOnce(t.Context())
	if n := len(w.rec.named("theme:source-failed")); n != 1 {
		t.Fatalf("persist failure reported %d times over two polls, want 1", n)
	}

	w.s.path = good
	w.s.pollOnce(t.Context())
	got, err := w.s.Get("live")
	if err != nil {
		t.Fatalf("Get(live) error = %v", err)
	}
	if got.Name != "После сбоя" {
		t.Fatalf("reload was not retried once saving works again: %+v", got)
	}
	if n := len(w.rec.named("theme:source-failed")); n != 1 {
		t.Fatalf("recovery reported again: %d failures", n)
	}
}

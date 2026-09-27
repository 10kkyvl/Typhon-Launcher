package compat

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type recorder struct {
	mu      sync.Mutex
	reports []Report
	bodies  []string
}

func (r *recorder) add(t *testing.T, body []byte) {
	t.Helper()
	var report Report
	if err := json.Unmarshal(body, &report); err != nil {
		t.Fatalf("отчёт не разбирается: %v\n%s", err, body)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.reports = append(r.reports, report)
	r.bodies = append(r.bodies, string(body))
}

func (r *recorder) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.reports)
}

func newTestSharer(t *testing.T, allowed func() bool) (*Sharer, *Service, *recorder) {
	t.Helper()
	rec := &recorder{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
		}
		rec.add(t, body)
		w.WriteHeader(http.StatusAccepted)
	}))
	t.Cleanup(srv.Close)

	dir := t.TempDir()
	journal, err := NewServiceAt(filepath.Join(dir, "compat.json"))
	if err != nil {
		t.Fatalf("NewServiceAt: %v", err)
	}
	stats := NewStatsAt(filepath.Join(dir, "compat-stats.json"))

	sharer, err := NewSharer(journal, stats, srv.URL, "client-uuid", "0.3.1", allowed,
		func() Env { return Env{OSVersion: "15.6", CrossOver: "26.3", Chip: "Apple M4"} },
		func(localID string) (Build, bool) {
			return Build{GameID: map[string]string{"g1": "232567", "g2": "1020"}[localID], Repacker: "fitgirl"}, true
		})
	if err != nil {
		t.Fatalf("NewSharer: %v", err)
	}
	return sharer, journal, rec
}

// Главный инвариант: без согласия наружу не уходит ничего, сколько бы журнал ни
// накопил.
func TestNothingIsSentWithoutConsent(t *testing.T) {
	sharer, journal, rec := newTestSharer(t, func() bool { return false })
	journal.RecordSession("g1", 30*time.Minute, true)

	sharer.sendOnce(context.Background())

	if rec.count() != 0 {
		t.Fatalf("отправлено %d отчётов без согласия:\n%s", rec.count(), rec.bodies)
	}
}

// Согласие, отозванное между кругами цикла, обязано остановить отправку на
// следующем же круге, а не на следующем запуске.
func TestConsentIsAskedBeforeEverySend(t *testing.T) {
	var allow bool
	sharer, journal, rec := newTestSharer(t, func() bool { return allow })
	journal.RecordSession("g1", 30*time.Minute, true)

	sharer.sendOnce(context.Background())
	if rec.count() != 0 {
		t.Fatal("отправлено до согласия")
	}

	allow = true
	sharer.sendOnce(context.Background())
	if rec.count() != 1 {
		t.Fatalf("после согласия отправлено %d, want 1", rec.count())
	}

	allow = false
	journal.RecordSession("g2", 30*time.Minute, true)
	sharer.sendOnce(context.Background())
	if rec.count() != 1 {
		t.Fatalf("после отзыва согласия отправлено %d, want 1", rec.count())
	}
}

func TestEmptyJournalIsNotSent(t *testing.T) {
	sharer, _, rec := newTestSharer(t, func() bool { return true })
	sharer.sendOnce(context.Background())
	if rec.count() != 0 {
		t.Fatalf("отправлен пустой отчёт: %s", rec.bodies)
	}
}

func TestUnchangedJournalIsNotResent(t *testing.T) {
	sharer, journal, rec := newTestSharer(t, func() bool { return true })
	journal.RecordSession("g1", 30*time.Minute, true)

	sharer.sendOnce(context.Background())
	sharer.sendOnce(context.Background())
	if rec.count() != 1 {
		t.Fatalf("отправлено %d отчётов, want 1", rec.count())
	}

	journal.RecordLaunchFailure("g2", "library.launch_failed", "не поехало")
	journal.RecordLaunchFailure("g2", "library.launch_failed", "не поехало")
	sharer.sendOnce(context.Background())
	if rec.count() != 2 {
		t.Fatalf("изменившийся журнал не отправлен: %d отчётов", rec.count())
	}
}

// Неудачная отправка не должна считаться отправленной: иначе один отказ сервера
// похоронил бы вердикт до следующего изменения журнала.
func TestFailedSendIsRetriedNextRound(t *testing.T) {
	rec := &recorder{}
	var fail bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
		}
		rec.add(t, body)
		if fail {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	dir := t.TempDir()
	journal, err := NewServiceAt(filepath.Join(dir, "compat.json"))
	if err != nil {
		t.Fatalf("NewServiceAt: %v", err)
	}
	sharer, err := NewSharer(journal, NewStatsAt(filepath.Join(dir, "s.json")), srv.URL,
		"c", "0.3.1", func() bool { return true },
		func() Env { return Env{OSVersion: "15.6", Chip: "apple_m4"} },
		func(string) (Build, bool) { return Build{GameID: "1"}, true })
	if err != nil {
		t.Fatalf("NewSharer: %v", err)
	}
	journal.RecordSession("g1", 30*time.Minute, true)

	fail = true
	sharer.sendOnce(context.Background())
	fail = false
	sharer.sendOnce(context.Background())

	if rec.count() != 2 {
		t.Fatalf("попыток отправки %d, want 2", rec.count())
	}
}

func TestStatsAreFetchedAndCached(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		hits++
		if req.Header.Get("If-None-Match") == `W/"v1"` {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", `W/"v1"`)
		if err := json.NewEncoder(w).Encode(Snapshot{
			Games: []Shared{{GameID: "232567", Repacker: AnyRepacker, Works: 9, Total: 10}},
		}); err != nil {
			t.Errorf("encode snapshot: %v", err)
		}
	}))
	defer srv.Close()

	dir := t.TempDir()
	journal, err := NewServiceAt(filepath.Join(dir, "compat.json"))
	if err != nil {
		t.Fatalf("NewServiceAt: %v", err)
	}
	stats := NewStatsAt(filepath.Join(dir, "compat-stats.json"))
	sharer, err := NewSharer(journal, stats, srv.URL, "c", "0.3.1",
		func() bool { return true }, func() Env { return Env{} },
		func(string) (Build, bool) { return Build{}, false })
	if err != nil {
		t.Fatalf("NewSharer: %v", err)
	}

	sharer.refreshStats(context.Background())
	got, ok := stats.Game("232567")
	if !ok || got.Total != 10 {
		t.Fatalf("агрегат не применился: %+v, ok = %v", got, ok)
	}

	// Второй заход обязан уйти с ETag и не тронуть кэш.
	sharer.refreshStats(context.Background())
	if hits != 2 {
		t.Fatalf("запросов %d, want 2", hits)
	}
	if stats.Len() != 1 {
		t.Fatalf("кэш после 304 = %d строк, want 1", stats.Len())
	}
}

// Общая цифра читается без согласия: забрать чужую статистику — не то же самое,
// что отдать свою.
func TestStatsAreFetchedWithoutConsent(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		if err := json.NewEncoder(w).Encode(Snapshot{
			Games: []Shared{{GameID: "1", Repacker: AnyRepacker, Works: 5, Total: 5}},
		}); err != nil {
			t.Errorf("encode snapshot: %v", err)
		}
	}))
	defer srv.Close()

	dir := t.TempDir()
	journal, err := NewServiceAt(filepath.Join(dir, "compat.json"))
	if err != nil {
		t.Fatalf("NewServiceAt: %v", err)
	}
	stats := NewStatsAt(filepath.Join(dir, "compat-stats.json"))
	sharer, err := NewSharer(journal, stats, srv.URL, "c", "0.3.1",
		func() bool { return false }, func() Env { return Env{} },
		func(string) (Build, bool) { return Build{}, false })
	if err != nil {
		t.Fatalf("NewSharer: %v", err)
	}

	sharer.refreshStats(context.Background())
	if hits != 1 || stats.Len() != 1 {
		t.Fatalf("запросов %d, строк %d", hits, stats.Len())
	}
}

// Отказ от отправки не должен быть немым: на маке без версии системы иначе не
// видно, почему статистика не уходит. Но и шуметь каждые десять минут ему
// незачем — хватает одной строки за запуск.
func TestSkippedReportIsLoggedOnce(t *testing.T) {
	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })

	srv, cs := newContractServer(t)
	dir := t.TempDir()
	journal, err := NewServiceAt(filepath.Join(dir, "compat.json"))
	if err != nil {
		t.Fatalf("NewServiceAt: %v", err)
	}
	journal.RecordSession("g1", 30*time.Minute, true)
	sharer, err := NewSharer(journal, NewStatsAt(filepath.Join(dir, "compat-stats.json")), srv.URL,
		"3f1c2b8e-4d5a-4f6b-9c7d-8e9f0a1b2c3d", "0.7.2",
		func() bool { return true },
		func() Env { return windowsEnv },
		func(string) (Build, bool) { return Build{GameID: "376206"}, true })
	if err != nil {
		t.Fatalf("NewSharer: %v", err)
	}

	for range 3 {
		sharer.sendOnce(context.Background())
	}

	if accepted, rejected := cs.snapshot(); len(accepted)+len(rejected) != 0 {
		t.Fatalf("отправлено без версии macOS: принято %d, отклонено %v", len(accepted), rejected)
	}
	if n := strings.Count(logs.String(), "no macOS version"); n != 1 {
		t.Fatalf("строк о пропуске %d, want 1:\n%s", n, logs.String())
	}
}

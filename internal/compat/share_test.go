package compat

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"typhon/internal/account"
	"typhon/internal/app"
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
		func() (Env, error) { return Env{OSVersion: "15.6", CrossOver: "26.3", Chip: "Apple M4"}, nil },
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
		func() (Env, error) { return Env{OSVersion: "15.6", Chip: "apple_m4"}, nil },
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
		func() bool { return true }, func() (Env, error) { return Env{}, nil },
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
		func() bool { return false }, func() (Env, error) { return Env{}, nil },
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
		func() (Env, error) { return windowsEnv, nil },
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

func newSharerFor(t *testing.T, url string, env func() (Env, error), resolve func(string) (Build, bool)) (*Sharer, *Service) {
	t.Helper()
	dir := t.TempDir()
	journal, err := NewServiceAt(filepath.Join(dir, "compat.json"))
	if err != nil {
		t.Fatalf("NewServiceAt: %v", err)
	}
	sharer, err := NewSharer(journal, NewStatsAt(filepath.Join(dir, "compat-stats.json")), url, "c", "0.3.1",
		func() bool { return true }, env, resolve)
	if err != nil {
		t.Fatalf("NewSharer: %v", err)
	}
	return sharer, journal
}

func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	return &logs
}

func knownEnv() (Env, error) { return macEnv, nil }

// Сервер видит по этим заголовкам, какая версия лаунчера шлёт отчёт и под какую
// ОС она собрана; без них отчёт и запрос статистики выглядели бы чужим клиентом.
func TestRequestsCarryClientIdentity(t *testing.T) {
	var mu sync.Mutex
	seen := map[string]http.Header{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		mu.Lock()
		seen[req.Method] = req.Header.Clone()
		mu.Unlock()
		if req.Method == http.MethodGet {
			if err := json.NewEncoder(w).Encode(Snapshot{}); err != nil {
				t.Errorf("encode snapshot: %v", err)
			}
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	sharer, journal := newSharerFor(t, srv.URL, knownEnv, func(string) (Build, bool) { return Build{GameID: "1"}, true })
	journal.RecordSession("g1", 30*time.Minute, true)

	sharer.sendOnce(context.Background())
	sharer.refreshStats(context.Background())

	mu.Lock()
	defer mu.Unlock()
	for _, method := range []string{http.MethodPost, http.MethodGet} {
		header, ok := seen[method]
		if !ok {
			t.Fatalf("запроса %s не было", method)
		}
		if got := header.Get("User-Agent"); got != account.UserAgent || !strings.HasPrefix(got, "Typhon/") {
			t.Errorf("%s: User-Agent = %q, want %q", method, got, account.UserAgent)
		}
		if got := header.Get("X-Typhon-Version"); got != app.Version || got == "" {
			t.Errorf("%s: X-Typhon-Version = %q, want %q", method, got, app.Version)
		}
	}
	if got := seen[http.MethodPost].Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", got)
	}
}

// 400 и 413 говорят про само содержимое отчёта: со второй попытки оно не
// станет принятым. Повторять его каждые десять минут значило бы вечно
// стучаться с тем же отказом; новый отчёт, напротив, уходит сразу.
func TestRejectedReportIsNotRepeatedUntilItChanges(t *testing.T) {
	for _, status := range []int{http.StatusBadRequest, http.StatusRequestEntityTooLarge} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			logs := captureLogs(t)
			var requests atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				requests.Add(1)
				w.WriteHeader(status)
			}))
			defer srv.Close()
			sharer, journal := newSharerFor(t, srv.URL, knownEnv, func(localID string) (Build, bool) {
				return Build{GameID: map[string]string{"g1": "1", "g2": "2"}[localID]}, true
			})
			journal.RecordSession("g1", 30*time.Minute, true)

			for range 3 {
				sharer.sendOnce(context.Background())
			}
			if n := requests.Load(); n != 1 {
				t.Fatalf("запросов %d, want 1: отклонённый отчёт ушёл повторно", n)
			}
			if n := strings.Count(logs.String(), "rejected by the server"); n != 1 {
				t.Fatalf("строк об отказе %d, want 1:\n%s", n, logs.String())
			}

			journal.RecordSession("g2", 30*time.Minute, true)
			sharer.sendOnce(context.Background())
			if n := requests.Load(); n != 2 {
				t.Fatalf("запросов %d, want 2: изменившийся отчёт не ушёл", n)
			}
		})
	}
}

// Отчёт из нескольких запросов считается отправленным, только когда принят
// последний: оборвавшийся на середине уходит заново целиком, а принятая часть
// от этого не портится, потому что сервер перезаписывает строки по ключу.
func TestPartiallyDeliveredReportIsSentAgainInFull(t *testing.T) {
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if requests.Add(1) == 2 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	var builds map[string]Build
	sharer, journal := newSharerFor(t, srv.URL, knownEnv, func(localID string) (Build, bool) {
		b, ok := builds[localID]
		return b, ok
	})
	builds = fillJournal(journal, maxReportGames+100, false)

	sharer.sendOnce(context.Background())
	if n := requests.Load(); n != 2 {
		t.Fatalf("после обрыва запросов %d, want 2", n)
	}
	sharer.sendOnce(context.Background())
	if n := requests.Load(); n != 4 {
		t.Fatalf("после повтора запросов %d, want 4: отчёт ушёл не весь", n)
	}
	sharer.sendOnce(context.Background())
	if n := requests.Load(); n != 4 {
		t.Fatalf("принятый отчёт ушёл снова: запросов %d, want 4", n)
	}
}

// Окружение, которое не определилось, не превращается в пустые поля отчёта:
// отчёт не уходит, а причина попадает в лог один раз за запуск.
func TestEnvErrorSkipsTheReportAndIsLoggedOnce(t *testing.T) {
	logs := captureLogs(t)
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	cause := errors.New("sysctl hw.memsize: operation not permitted")
	sharer, journal := newSharerFor(t, srv.URL,
		func() (Env, error) { return Env{}, cause },
		func(string) (Build, bool) { return Build{GameID: "1"}, true })
	journal.RecordSession("g1", 30*time.Minute, true)

	for range 3 {
		sharer.sendOnce(context.Background())
	}

	if n := requests.Load(); n != 0 {
		t.Fatalf("запросов %d, want 0: отчёт ушёл без окружения", n)
	}
	out := logs.String()
	if n := strings.Count(out, "environment unknown"); n != 1 {
		t.Fatalf("строк о пропуске %d, want 1:\n%s", n, out)
	}
	if !strings.Contains(out, cause.Error()) {
		t.Fatalf("в логе нет причины %q:\n%s", cause, out)
	}
}

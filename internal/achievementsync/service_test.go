package achievementsync

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"typhon/internal/achievements"
	"typhon/internal/library"
	"typhon/internal/storage"
	"typhon/internal/uierr"
)

var errGameMissing = uierr.New("library.game_not_found", "игра не найдена")

type fakeGames struct {
	games map[string]library.Game
	err   error
}

func (f fakeGames) Find(id string) (library.Game, error) {
	if f.err != nil {
		return library.Game{}, f.err
	}
	g, ok := f.games[id]
	if !ok {
		return library.Game{}, errGameMissing
	}
	return g, nil
}

type server struct {
	mu       sync.Mutex
	status   int
	bodies   []request
	auth     []string
	response func(n int) int
}

func (s *server) handler(t *testing.T) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/me/achievements" || r.Method != http.MethodPost {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read: %v", err)
		}
		var req request
		if err := json.Unmarshal(raw, &req); err != nil {
			t.Errorf("decode %s: %v", raw, err)
		}
		s.mu.Lock()
		s.bodies = append(s.bodies, req)
		s.auth = append(s.auth, r.Header.Get("Authorization"))
		status := s.status
		if s.response != nil {
			status = s.response(len(s.bodies))
		}
		s.mu.Unlock()
		w.WriteHeader(status)
		if status >= 400 {
			if _, err := w.Write([]byte(`{"error":{"code":"bad_request"}}`)); err != nil {
				t.Errorf("write: %v", err)
			}
		}
	}
}

func (s *server) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.bodies)
}

type rig struct {
	svc     *Service
	srv     *server
	dir     string
	consent bool
	token   string
	account string
	now     time.Time
}

func newRig(t *testing.T) *rig {
	t.Helper()
	r := &rig{srv: &server{status: http.StatusNoContent}, dir: t.TempDir(), consent: true, token: "tok", account: "user-a", now: t0}
	ts := httptest.NewServer(r.srv.handler(t))
	t.Cleanup(ts.Close)
	svc, err := New(Config{
		ConfigDir: r.dir,
		BaseURL:   ts.URL,
		Token:     func() (string, error) { return r.token, nil },
		AccountID: func(string) (string, error) { return r.account, nil },
		Consent:   func() bool { return r.consent },
		Games: fakeGames{games: map[string]library.Game{
			"g1":     {ID: "g1", CanonicalGameID: "c1"},
			"hidden": {ID: "hidden", CanonicalGameID: "c1", Hidden: true},
			"noigdb": {ID: "noigdb", CanonicalGameID: "c2"},
			"g2":     {ID: "g2", CanonicalGameID: "c3"},
		}},
		IGDBID: func(id string) string {
			switch id {
			case "c1":
				return "1942"
			case "c3":
				return "2000"
			}
			return ""
		},
		Clock: func() time.Time { return r.now },
	})
	if err != nil {
		t.Fatal(err)
	}
	r.svc = svc
	return r
}

func unlock(key string, at time.Time) achievements.Unlock {
	return achievements.Unlock{Key: key, At: at}
}

func TestReportGating(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(r *rig)
		game   string
		app    string
		want   int
	}{
		{"queued", func(*rig) {}, "g1", "620", 1},
		{"no consent", func(r *rig) { r.consent = false }, "g1", "620", 0},
		{"signed out", func(r *rig) { r.token = "" }, "g1", "620", 0},
		{"hidden game", func(*rig) {}, "hidden", "620", 0},
		{"no igdb id", func(*rig) {}, "noigdb", "620", 0},
		{"no steam app id", func(*rig) {}, "g1", "", 0},
		{"unknown game", func(*rig) {}, "gone", "620", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newRig(t)
			tt.mutate(r)
			if err := r.svc.Report(tt.game, tt.app, []achievements.Unlock{unlock("A", t0)}); err != nil {
				t.Fatal(err)
			}
			if got := len(r.svc.box.items); got != tt.want {
				t.Fatalf("queued = %d, want %d", got, tt.want)
			}
			if tt.want == 0 {
				if _, err := os.Stat(filepath.Join(r.dir, storeFile)); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("outbox file written without a report: %v", err)
				}
			}
		})
	}
}

func TestReportSkipsUnlocksOutsideTheWindow(t *testing.T) {
	r := newRig(t)
	err := r.svc.Report("g1", "620", []achievements.Unlock{
		unlock("OLD", t0.Add(-maxAge-time.Minute)),
		unlock("FUTURE", t0.Add(2*time.Hour)),
		unlock("", t0),
		unlock("OK", t0.Add(-time.Hour)),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.svc.box.items) != 1 {
		t.Fatalf("queued %+v", r.svc.box.items)
	}
}

func TestReportErrors(t *testing.T) {
	r := newRig(t)
	r.svc.games = fakeGames{err: errors.New("library down")}
	if err := r.svc.Report("g1", "620", []achievements.Unlock{unlock("A", t0)}); err == nil {
		t.Fatal("library failure was swallowed")
	}
	r2 := newRig(t)
	r2.svc.token = func() (string, error) { return "", errors.New("keychain locked") }
	if err := r2.svc.Report("g1", "620", []achievements.Unlock{unlock("A", t0)}); err == nil {
		t.Fatal("token failure was swallowed")
	}
}

func TestFlushStatusSemantics(t *testing.T) {
	tests := []struct {
		name     string
		status   int
		wantLeft int
		wantErr  bool
	}{
		{"204 drops", http.StatusNoContent, 0, false},
		{"200 drops", http.StatusOK, 0, false},
		{"400 drops", http.StatusBadRequest, 0, false},
		{"401 keeps", http.StatusUnauthorized, 1, true},
		{"408 keeps", http.StatusRequestTimeout, 1, true},
		{"429 keeps", http.StatusTooManyRequests, 1, true},
		{"500 keeps", http.StatusInternalServerError, 1, true},
		{"503 keeps", http.StatusServiceUnavailable, 1, true},
		{"404 drops", http.StatusNotFound, 0, false},
		{"405 drops", http.StatusMethodNotAllowed, 0, false},
		{"413 drops", http.StatusRequestEntityTooLarge, 0, false},
		{"422 drops", http.StatusUnprocessableEntity, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newRig(t)
			r.srv.status = tt.status
			if err := r.svc.Report("g1", "620", []achievements.Unlock{unlock("A", t0)}); err != nil {
				t.Fatal(err)
			}
			err := r.svc.Flush(context.Background())
			if (err != nil) != tt.wantErr {
				t.Fatalf("flush err = %v", err)
			}
			if tt.wantErr {
				var retry *RetryableError
				if !errors.As(err, &retry) || retry.Status != tt.status {
					t.Fatalf("err = %#v", err)
				}
			}
			if len(r.svc.box.items) != tt.wantLeft {
				t.Fatalf("left = %d, want %d", len(r.svc.box.items), tt.wantLeft)
			}
			reopened := openOutbox(r.dir, storage.Save)
			if len(reopened.items) != tt.wantLeft {
				t.Fatalf("disk holds %d, want %d", len(reopened.items), tt.wantLeft)
			}
			if got := r.srv.bodies[0]; got.IGDBID != 1942 || got.SteamAppID != 620 || len(got.Unlocks) != 1 || got.Unlocks[0].Key != "A" {
				t.Fatalf("sent %+v", got)
			}
			if r.srv.auth[0] != "Bearer tok" {
				t.Fatalf("auth = %q", r.srv.auth[0])
			}
		})
	}
}

func TestClientClassifiesFailures(t *testing.T) {
	r := newRig(t)
	r.srv.status = http.StatusBadRequest
	err := r.svc.client.report(context.Background(), "tok", request{IGDBID: 1, SteamAppID: 2, Unlocks: []Unlock{{Key: "A", UnlockedAt: t0}}})
	var perm *PermanentError
	if !errors.As(err, &perm) || perm.Status != 400 || perm.Code != "bad_request" {
		t.Fatalf("400 = %#v", err)
	}
	var retry *RetryableError
	if errors.As(err, &retry) {
		t.Fatal("400 classified as retryable")
	}
	r.srv.status = http.StatusServiceUnavailable
	err = r.svc.client.report(context.Background(), "tok", request{})
	if !errors.As(err, &retry) || retry.Stop || retry.Code != "bad_request" {
		t.Fatalf("503 = %#v", err)
	}
	r.srv.status = http.StatusUnauthorized
	err = r.svc.client.report(context.Background(), "tok", request{})
	if !errors.As(err, &retry) || !retry.Stop {
		t.Fatalf("401 = %#v", err)
	}
	if err := r.svc.client.report(context.Background(), "", request{}); !errors.Is(err, ErrNotSignedIn) {
		t.Fatalf("signed out = %v", err)
	}
}

func TestFlushNetworkFailureKeepsEntries(t *testing.T) {
	r := newRig(t)
	if err := r.svc.Report("g1", "620", []achievements.Unlock{unlock("A", t0)}); err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(http.NotFoundHandler())
	r.svc.client.baseURL = ts.URL
	ts.Close()
	err := r.svc.Flush(context.Background())
	var retry *RetryableError
	if !errors.As(err, &retry) || retry.Status != 0 {
		t.Fatalf("err = %#v", err)
	}
	if len(r.svc.box.items) != 1 {
		t.Fatal("entry lost on network failure")
	}
}

func TestFlushDropsExpiredWithoutSending(t *testing.T) {
	r := newRig(t)
	if err := r.svc.Report("g1", "620", []achievements.Unlock{unlock("A", t0)}); err != nil {
		t.Fatal(err)
	}
	r.now = t0.Add(maxAge + time.Hour)
	if err := r.svc.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	if r.srv.count() != 0 || len(r.svc.box.items) != 0 {
		t.Fatalf("sent=%d left=%d", r.srv.count(), len(r.svc.box.items))
	}
}

func TestFlushDiscardsTheQueueWithoutConsentOrSession(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(r *rig)
	}{
		{"consent withdrawn", func(r *rig) { r.consent = false }},
		{"signed out", func(r *rig) { r.token = "" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newRig(t)
			if err := r.svc.Report("g1", "620", []achievements.Unlock{unlock("A", t0)}); err != nil {
				t.Fatal(err)
			}
			tt.mutate(r)
			if err := r.svc.Flush(context.Background()); err != nil || r.srv.count() != 0 {
				t.Fatalf("flush: %v, sent %d", err, r.srv.count())
			}
			if len(r.svc.box.items) != 0 {
				t.Fatalf("queue kept: %+v", r.svc.box.items)
			}
			if again := openOutbox(r.dir, storage.Save); len(again.items) != 0 {
				t.Fatalf("disk kept: %+v", again.items)
			}
		})
	}
}

func TestFlushRechecksConsentBeforeEveryBatch(t *testing.T) {
	r := newRig(t)
	if err := r.svc.Report("g1", "620", manyUnlocks(maxBatch*2)); err != nil {
		t.Fatal(err)
	}
	calls := 0
	r.svc.consent = func() bool {
		calls++
		return calls <= 2
	}
	if err := r.svc.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	if r.srv.count() != 1 {
		t.Fatalf("requests = %d, want 1", r.srv.count())
	}
	if len(r.svc.box.items) != 0 {
		t.Fatalf("the rest was kept after consent was withdrawn: %d", len(r.svc.box.items))
	}
}

func manyUnlocks(n int) []achievements.Unlock {
	out := make([]achievements.Unlock, 0, n)
	for i := range n {
		out = append(out, unlock("K"+strconv.Itoa(i), t0.Add(-time.Duration(i)*time.Second)))
	}
	return out
}

func TestFlushBatchesByLimit(t *testing.T) {
	r := newRig(t)
	if err := r.svc.Report("g1", "620", manyUnlocks(maxBatch+5)); err != nil {
		t.Fatal(err)
	}
	if err := r.svc.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	if r.srv.count() != 2 || len(r.srv.bodies[0].Unlocks) != maxBatch || len(r.srv.bodies[1].Unlocks) != 5 {
		t.Fatalf("batches: %d", r.srv.count())
	}
}

func TestFlushStopsAtFirstRetryableBatchAndKeepsTheRest(t *testing.T) {
	r := newRig(t)
	r.srv.response = func(n int) int {
		if n == 1 {
			return http.StatusNoContent
		}
		return http.StatusServiceUnavailable
	}
	if err := r.svc.Report("g1", "620", manyUnlocks(maxBatch*2)); err != nil {
		t.Fatal(err)
	}
	if err := r.svc.Flush(context.Background()); err == nil {
		t.Fatal("expected a retryable error")
	}
	if len(r.svc.box.items) != maxBatch || r.srv.count() != 2 {
		t.Fatalf("left = %d, requests = %d", len(r.svc.box.items), r.srv.count())
	}
}

func TestFlushCancelledContext(t *testing.T) {
	r := newRig(t)
	if err := r.svc.Report("g1", "620", []achievements.Unlock{unlock("A", t0)}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := r.svc.Flush(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v", err)
	}
	if r.srv.count() != 0 || len(r.svc.box.items) != 1 {
		t.Fatal("cancelled flush sent or dropped data")
	}
}

func TestBrokenOutboxRefusesReportAndFlush(t *testing.T) {
	r := newRig(t)
	if err := os.WriteFile(filepath.Join(r.dir, storeFile), []byte("garbage"), 0o600); err != nil {
		t.Fatal(err)
	}
	r.svc.box = openOutbox(r.dir, storage.Save)
	if err := r.svc.Report("g1", "620", []achievements.Unlock{unlock("A", t0)}); !errors.Is(err, ErrOutboxBroken) {
		t.Fatalf("report = %v", err)
	}
	if err := r.svc.Flush(context.Background()); !errors.Is(err, ErrOutboxBroken) {
		t.Fatalf("flush = %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(r.dir, storeFile))
	if err != nil || string(raw) != "garbage" {
		t.Fatalf("the unreadable outbox was replaced: %q %v", raw, err)
	}
}

func TestRunFlushesOnReportAndStopsOnShutdown(t *testing.T) {
	r := newRig(t)
	if err := r.svc.ServiceStartup(t.Context(), application.ServiceOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := r.svc.Report("g1", "620", []achievements.Unlock{unlock("A", t0)}); err != nil {
		t.Fatal(err)
	}
	deadline := time.After(10 * time.Second)
	for {
		r.svc.mu.Lock()
		left := len(r.svc.box.items)
		r.svc.mu.Unlock()
		if left == 0 && r.srv.count() == 1 {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("not flushed: left %d sent %d", left, r.srv.count())
		case <-time.After(5 * time.Millisecond):
		}
	}
	if err := r.svc.ServiceShutdown(); err != nil {
		t.Fatal(err)
	}
}

func TestNewValidatesConfig(t *testing.T) {
	ok := func() Config {
		return Config{
			ConfigDir: t.TempDir(), BaseURL: "https://api.test",
			Token:     func() (string, error) { return "t", nil },
			AccountID: func(string) (string, error) { return "a", nil },
			Consent:   func() bool { return true }, Games: fakeGames{}, IGDBID: func(string) string { return "" },
		}
	}
	if _, err := New(ok()); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*Config){
		"empty dir":     func(c *Config) { c.ConfigDir = "" },
		"relative dir":  func(c *Config) { c.ConfigDir = "rel" },
		"no consent":    func(c *Config) { c.Consent = nil },
		"no games":      func(c *Config) { c.Games = nil },
		"no igdb":       func(c *Config) { c.IGDBID = nil },
		"no account":    func(c *Config) { c.AccountID = nil },
		"no token":      func(c *Config) { c.Token = nil },
		"insecure base": func(c *Config) { c.BaseURL = "http://api.example.com" },
	} {
		cfg := ok()
		mutate(&cfg)
		if _, err := New(cfg); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
}

func TestFlushSendsOnlyTheCurrentAccountsEntries(t *testing.T) {
	r := newRig(t)
	if err := r.svc.Report("g1", "620", []achievements.Unlock{unlock("A1", t0)}); err != nil {
		t.Fatal(err)
	}
	r.account = "user-b"
	if err := r.svc.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	if r.srv.count() != 0 || len(r.svc.box.items) != 1 {
		t.Fatalf("account A's unlock went out under B: sent %d, left %d", r.srv.count(), len(r.svc.box.items))
	}
	if err := r.svc.Report("g1", "620", []achievements.Unlock{unlock("B1", t0)}); err != nil {
		t.Fatal(err)
	}
	if err := r.svc.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	if r.srv.count() != 1 || len(r.srv.bodies[0].Unlocks) != 1 || r.srv.bodies[0].Unlocks[0].Key != "B1" {
		t.Fatalf("B sent %+v", r.srv.bodies)
	}
	if len(r.svc.box.items) != 1 || r.svc.box.items[0].Key != "A1" {
		t.Fatalf("A's entry should wait for A: %+v", r.svc.box.items)
	}
	r.account = "user-a"
	if err := r.svc.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	if r.srv.count() != 2 || r.srv.bodies[1].Unlocks[0].Key != "A1" || len(r.svc.box.items) != 0 {
		t.Fatalf("A back: sent %+v, left %+v", r.srv.bodies, r.svc.box.items)
	}
}

func TestDedupeIsPerOwner(t *testing.T) {
	r := newRig(t)
	for _, acc := range []string{"user-a", "user-b", "user-a"} {
		r.account = acc
		if err := r.svc.Report("g1", "620", []achievements.Unlock{unlock("SAME", t0)}); err != nil {
			t.Fatal(err)
		}
	}
	if len(r.svc.box.items) != 2 || r.svc.box.items[0].Owner == r.svc.box.items[1].Owner {
		t.Fatalf("items = %+v", r.svc.box.items)
	}
}

func TestOwnerIsNotTheRawTokenOrAccountID(t *testing.T) {
	r := newRig(t)
	if err := r.svc.Report("g1", "620", []achievements.Unlock{unlock("A", t0)}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(r.dir, storeFile))
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"tok", "user-a"} {
		if strings.Contains(string(raw), secret) {
			t.Fatalf("outbox file contains %q: %s", secret, raw)
		}
	}
}

func TestEntryWithoutOwnerIsDroppedOnLoadAndNeverSent(t *testing.T) {
	r := newRig(t)
	legacy := state{Entries: []entry{{IGDBID: 1942, SteamAppID: 620, Key: "OLD", UnlockedAt: t0}}}
	if err := storage.Save(filepath.Join(r.dir, storeFile), storeVersion, legacy); err != nil {
		t.Fatal(err)
	}
	r.svc.box = openOutbox(r.dir, storage.Save)
	if len(r.svc.box.items) != 0 {
		t.Fatalf("kept %+v", r.svc.box.items)
	}
	if err := r.svc.Flush(context.Background()); err != nil || r.srv.count() != 0 {
		t.Fatalf("flush: %v, sent %d", err, r.srv.count())
	}
}

func TestUnlocksSurviveAnUnknownAccountAndRebindOnceItResolves(t *testing.T) {
	r := newRig(t)
	known := false
	r.svc.account = func(string) (string, error) {
		if !known {
			return "", errors.New("not bound yet")
		}
		return r.account, nil
	}
	if err := r.svc.Report("g1", "620", []achievements.Unlock{unlock("A", t0)}); err != nil {
		t.Fatalf("an unknown account must not lose the unlock: %v", err)
	}
	if err := r.svc.Flush(context.Background()); err == nil || r.srv.count() != 0 {
		t.Fatalf("flush = %v, sent %d", err, r.srv.count())
	}
	if len(r.svc.box.items) != 1 {
		t.Fatal("unlock dropped while the account was unknown")
	}
	known = true
	if err := r.svc.Report("g1", "620", []achievements.Unlock{unlock("A", t0)}); err != nil {
		t.Fatal(err)
	}
	if err := r.svc.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	if r.srv.count() != 1 || len(r.srv.bodies[0].Unlocks) != 1 || len(r.svc.box.items) != 0 {
		t.Fatalf("sent %+v, left %+v", r.srv.bodies, r.svc.box.items)
	}
}

func TestPendingEntriesOfAnotherSessionAreNotSent(t *testing.T) {
	r := newRig(t)
	known := false
	r.svc.account = func(string) (string, error) {
		if !known {
			return "", errors.New("not bound yet")
		}
		return r.account, nil
	}
	if err := r.svc.Report("g1", "620", []achievements.Unlock{unlock("A", t0)}); err != nil {
		t.Fatal(err)
	}
	known = true
	r.token = "other-session"
	if err := r.svc.Flush(context.Background()); err != nil || r.srv.count() != 0 {
		t.Fatalf("flush = %v, sent %d", err, r.srv.count())
	}
	if len(r.svc.box.items) != 1 {
		t.Fatalf("items = %+v", r.svc.box.items)
	}
}

func TestFlushStopsWhenTheSessionChangesMidFlush(t *testing.T) {
	r := newRig(t)
	if err := r.svc.Report("g1", "620", manyUnlocks(maxBatch*2)); err != nil {
		t.Fatal(err)
	}
	calls := 0
	r.svc.token = func() (string, error) {
		calls++
		if calls <= 2 {
			return "tok", nil
		}
		return "tok-of-b", nil
	}
	err := r.svc.Flush(context.Background())
	if !errors.Is(err, errSessionChanged) {
		t.Fatalf("err = %v", err)
	}
	if r.srv.count() != 1 {
		t.Fatalf("requests = %d, want 1", r.srv.count())
	}
	for _, a := range r.srv.auth {
		if a != "Bearer tok" {
			t.Fatalf("batch sent with %q", a)
		}
	}
	if len(r.svc.box.items) != maxBatch {
		t.Fatalf("left = %d", len(r.svc.box.items))
	}
}

func TestServerErrorOnOneGameDoesNotBlockTheNext(t *testing.T) {
	r := newRig(t)
	r.srv.response = func(n int) int {
		if n == 1 {
			return http.StatusServiceUnavailable
		}
		return http.StatusNoContent
	}
	if err := r.svc.Report("g1", "620", []achievements.Unlock{unlock("A", t0)}); err != nil {
		t.Fatal(err)
	}
	if err := r.svc.Report("g2", "730", []achievements.Unlock{unlock("B", t0)}); err != nil {
		t.Fatal(err)
	}
	err := r.svc.Flush(context.Background())
	var retry *RetryableError
	if !errors.As(err, &retry) || retry.Status != 503 {
		t.Fatalf("err = %v", err)
	}
	if r.srv.count() != 2 {
		t.Fatalf("requests = %d", r.srv.count())
	}
	if len(r.svc.box.items) != 1 || r.svc.box.items[0].Key != "A" {
		t.Fatalf("left = %+v", r.svc.box.items)
	}
}

func TestServerErrorSkipsTheRestOfThatGame(t *testing.T) {
	r := newRig(t)
	r.srv.response = func(int) int { return http.StatusInternalServerError }
	if err := r.svc.Report("g1", "620", manyUnlocks(maxBatch*2)); err != nil {
		t.Fatal(err)
	}
	if err := r.svc.Flush(context.Background()); err == nil {
		t.Fatal("expected an error")
	}
	if r.srv.count() != 1 || len(r.svc.box.items) != maxBatch*2 {
		t.Fatalf("requests = %d, left = %d", r.srv.count(), len(r.svc.box.items))
	}
}

func TestSessionFailureStopsEveryGame(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusRequestTimeout, http.StatusTooManyRequests} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			r := newRig(t)
			r.srv.status = status
			if err := r.svc.Report("g1", "620", []achievements.Unlock{unlock("A", t0)}); err != nil {
				t.Fatal(err)
			}
			if err := r.svc.Report("g2", "730", []achievements.Unlock{unlock("B", t0)}); err != nil {
				t.Fatal(err)
			}
			if err := r.svc.Flush(context.Background()); err == nil {
				t.Fatal("expected an error")
			}
			if r.srv.count() != 1 || len(r.svc.box.items) != 2 {
				t.Fatalf("requests = %d, left = %d", r.srv.count(), len(r.svc.box.items))
			}
		})
	}
}

func TestReportValidatesKeys(t *testing.T) {
	r := newRig(t)
	long := strings.Repeat("я", maxKeyRunes)
	err := r.svc.Report("g1", "620", []achievements.Unlock{
		unlock(long, t0),
		unlock(long+"x", t0),
		unlock("bad\xff", t0),
		unlock("", t0),
		unlock("DUP", t0),
		unlock("DUP", t0.Add(-time.Minute)),
	})
	if err != nil {
		t.Fatal(err)
	}
	keys := map[string]int{}
	for _, it := range r.svc.box.items {
		keys[it.Key]++
	}
	if len(r.svc.box.items) != 2 || keys[long] != 1 || keys["DUP"] != 1 {
		t.Fatalf("queued %+v", r.svc.box.items)
	}
}

func TestReportSkipsUnlocksWithoutATime(t *testing.T) {
	r := newRig(t)
	if err := r.svc.Report("g1", "620", []achievements.Unlock{{Key: "NOTIME"}}); err != nil {
		t.Fatal(err)
	}
	if len(r.svc.box.items) != 0 {
		t.Fatalf("queued %+v", r.svc.box.items)
	}
	if _, err := os.Stat(filepath.Join(r.dir, storeFile)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("outbox written: %v", err)
	}
}

package metadata

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"typhon/internal/storage"
)

func newAttempts(t *testing.T, clock *testClock) (*attemptStore, string) {
	t.Helper()
	dir := t.TempDir()
	store, err := newAttemptStore(dir, clock.Now)
	if err != nil {
		t.Fatalf("attempt store: %v", err)
	}
	return store, dir
}

func TestUnmatchedGameWaitsOutTheBackoff(t *testing.T) {
	clock := newClock()
	store, _ := newAttempts(t, clock)

	if !store.due("prey", false) {
		t.Fatal("an unknown game must be due right away")
	}
	store.fail("prey", attemptUnmatched)
	if store.due("prey", false) {
		t.Fatal("an unmatched game was asked again immediately")
	}

	clock.advance(unmatchedBase - time.Second)
	if store.due("prey", false) {
		t.Fatal("backoff expired early")
	}
	clock.advance(2 * time.Second)
	if !store.due("prey", false) {
		t.Fatal("game never became due again")
	}
}

func TestBackoffGrowsWithRepeatedFailures(t *testing.T) {
	cases := []struct {
		name     string
		kind     attemptKind
		failures int
		want     time.Duration
	}{
		{"первая сетевая ошибка", attemptTransient, 1, transientBase},
		{"третья сетевая ошибка", attemptTransient, 3, 4 * transientBase},
		{"сетевой потолок", attemptTransient, 20, transientMax},
		{"игра не найдена", attemptUnmatched, 1, unmatchedBase},
		{"потолок для ненайденной", attemptUnmatched, 20, unmatchedMax},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := backoff(tc.kind, tc.failures); got != tc.want {
				t.Fatalf("backoff = %v, want %v", got, tc.want)
			}
		})
	}
}

// Открытая карточка игры не должна ждать шесть часов вместе с фоном: одна
// попытка на игру в четверть часа стоит дёшево и убирает пустую страницу.
func TestForegroundRetriesSoonerThanBackground(t *testing.T) {
	clock := newClock()
	store, _ := newAttempts(t, clock)
	store.fail("prey", attemptUnmatched)

	if store.due("prey", true) {
		t.Fatal("foreground retried instantly")
	}
	clock.advance(foregroundWindow + time.Second)
	if !store.due("prey", true) {
		t.Fatal("foreground still blocked after the short window")
	}
	if store.due("prey", false) {
		t.Fatal("background ignored its own backoff")
	}
}

func TestAttemptsSurviveARestart(t *testing.T) {
	clock := newClock()
	store, dir := newAttempts(t, clock)
	store.fail("prey", attemptUnmatched)
	if err := store.flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}

	reopened, err := newAttemptStore(dir, clock.Now)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if reopened.due("prey", false) {
		t.Fatal("backoff lost on restart: the launcher would re-ask the whole catalog")
	}

	clock.advance(unmatchedBase + time.Second)
	if !reopened.due("prey", false) {
		t.Fatal("restored record never expires")
	}
}

func TestSuccessClearsTheRecord(t *testing.T) {
	clock := newClock()
	store, dir := newAttempts(t, clock)
	store.fail("prey", attemptTransient)
	store.succeed("prey")

	if !store.due("prey", false) {
		t.Fatal("a resolved game stayed throttled")
	}
	if err := store.flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}
	reopened, err := newAttemptStore(dir, clock.Now)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if reopened.len() != 0 {
		t.Fatalf("records = %d, want the cleared game gone from disk", reopened.len())
	}
}

func TestExpiredRecordsAreDroppedOnFlush(t *testing.T) {
	clock := newClock()
	store, dir := newAttempts(t, clock)
	store.fail("prey", attemptTransient)
	clock.advance(transientMax + time.Hour)
	store.fail("dishonored", attemptTransient)

	if err := store.flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}
	reopened, err := newAttemptStore(dir, clock.Now)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if reopened.len() != 1 {
		t.Fatalf("records = %d, want only the live one", reopened.len())
	}
}

func TestBrokenAttemptsFileStopsTheStore(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"битый json", "{не json"},
		{"неизвестная версия", `{"version":99,"data":[]}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := storage.WriteAtomic(filepath.Join(dir, attemptsFileName), []byte(tc.body)); err != nil {
				t.Fatalf("seed: %v", err)
			}
			if _, err := newAttemptStore(dir, newClock().Now); err == nil {
				t.Fatal("store started on a broken file instead of reporting it")
			}
		})
	}
}

func TestEmptyDirIsRejected(t *testing.T) {
	if _, err := newAttemptStore("", newClock().Now); err == nil {
		t.Fatal("store accepted an empty directory")
	}
}

func TestMissingFileStartsEmpty(t *testing.T) {
	clock := newClock()
	store, _ := newAttempts(t, clock)
	if store.len() != 0 {
		t.Fatalf("records = %d, want an empty start", store.len())
	}
	if !store.due("prey", false) {
		t.Fatal("empty store throttled a game")
	}
}

func TestFlushReportsAWriteFailure(t *testing.T) {
	clock := newClock()
	dir := t.TempDir()
	store, err := newAttemptStore(dir, clock.Now)
	if err != nil {
		t.Fatalf("attempt store: %v", err)
	}
	store.fail("prey", attemptTransient)
	if err := os.RemoveAll(dir); err != nil {
		t.Fatalf("drop dir: %v", err)
	}
	if err := os.WriteFile(dir, []byte("не каталог"), 0o600); err != nil {
		t.Fatalf("block path: %v", err)
	}

	if err := store.flush(); err == nil {
		t.Fatal("flush reported success while the path is not writable")
	}
	if !store.dirtyNow() {
		t.Fatal("failed flush lost the pending records")
	}
}

func (s *attemptStore) dirtyNow() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.dirty
}

func TestTrimKeepsTheStoreBounded(t *testing.T) {
	clock := newClock()
	store, _ := newAttempts(t, clock)
	for i := range maxAttempts + 500 {
		store.fail(gameKey(i), attemptUnmatched)
	}
	if store.len() > maxAttempts {
		t.Fatalf("records = %d, want at most %d", store.len(), maxAttempts)
	}
}

func gameKey(i int) string {
	const digits = "0123456789abcdef"
	buf := make([]byte, 0, 8)
	for range 8 {
		buf = append(buf, digits[i%len(digits)])
		i /= len(digits)
	}
	return string(buf)
}

func TestDismissedGameIsNeverAskedAgain(t *testing.T) {
	clock := newClock()
	store, _ := newAttempts(t, clock)
	store.fail("prey", attemptUnmatched)
	store.dismiss("prey")

	clock.advance(unmatchedMax + 24*time.Hour)
	if store.due("prey", false) {
		t.Fatal("background asked about a game the user gave up on")
	}
	if store.due("prey", true) {
		t.Fatal("an open card asked about a game the user gave up on")
	}
}

func TestDismissalSurvivesARestart(t *testing.T) {
	clock := newClock()
	store, dir := newAttempts(t, clock)
	store.dismiss("prey")
	if err := store.flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}

	clock.advance(unmatchedMax + 24*time.Hour)
	reopened, err := newAttemptStore(dir, clock.Now)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if reopened.due("prey", true) {
		t.Fatal("dismissal lost on restart: the card asks again after every launch")
	}
}

func TestResumeClearsTheDismissal(t *testing.T) {
	clock := newClock()
	store, _ := newAttempts(t, clock)
	store.fail("prey", attemptUnmatched)
	store.dismiss("prey")
	store.resume("prey")

	if !store.due("prey", false) {
		t.Fatal("an explicit retry stayed blocked by the old dismissal")
	}
	if rec, ok := store.state("prey"); ok {
		t.Fatalf("record = %+v, want it gone after an explicit retry", rec)
	}
}

func TestRestoreRollsBackAFailedDismissal(t *testing.T) {
	cases := []struct {
		name  string
		seed  func(*attemptStore)
		wantK attemptKind
		wantR bool
	}{
		{"без прежней записи", func(*attemptStore) {}, "", false},
		{"поверх неудачной попытки", func(s *attemptStore) { s.fail("prey", attemptTransient) }, attemptTransient, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			clock := newClock()
			store, _ := newAttempts(t, clock)
			tc.seed(store)

			prev, existed := store.dismiss("prey")
			store.restore("prey", prev, existed)

			rec, ok := store.state("prey")
			if ok != tc.wantR {
				t.Fatalf("record present = %v, want %v", ok, tc.wantR)
			}
			if ok && (rec.Dismissed || rec.Kind != tc.wantK) {
				t.Fatalf("record = %+v, want the pre-dismissal state", rec)
			}
		})
	}
}

func TestDismissedRecordsSurviveTrim(t *testing.T) {
	clock := newClock()
	store, _ := newAttempts(t, clock)
	store.dismiss("prey")
	for i := range maxAttempts + 500 {
		store.fail(gameKey(i), attemptUnmatched)
	}

	if store.due("prey", true) {
		t.Fatal("trim threw away the user's decision")
	}
}

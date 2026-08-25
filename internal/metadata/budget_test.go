package metadata

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"

	"typhon/internal/catalog"
)

type testClock struct {
	mu  sync.Mutex
	now time.Time
}

func newClock() *testClock {
	return &testClock{now: time.Date(2026, 8, 23, 3, 0, 0, 0, time.UTC)}
}

func (c *testClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *testClock) advance(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	c.mu.Unlock()
}

func TestBudgetSpendsTheBurstThenAsksToWait(t *testing.T) {
	clock := newClock()
	b := newBudget(clock.Now)

	for i := range int(burst) {
		if wait, ok := b.reserve(classBackground); !ok {
			t.Fatalf("token %d refused with wait %v, want the burst to pass", i, wait)
		}
	}

	wait, ok := b.reserve(classBackground)
	if ok {
		t.Fatal("token handed out beyond the burst")
	}
	if want := time.Duration(1 / startRate * float64(time.Second)); wait < want {
		t.Fatalf("wait = %v, want at least one token interval %v", wait, want)
	}
}

func TestBudgetRefillsWithTime(t *testing.T) {
	clock := newClock()
	b := newBudget(clock.Now)
	for range int(burst) {
		if _, ok := b.reserve(classBackground); !ok {
			t.Fatal("burst refused")
		}
	}

	clock.advance(2 * time.Second)
	for i := range int(2 * startRate) {
		if _, ok := b.reserve(classBackground); !ok {
			t.Fatalf("refilled token %d refused", i)
		}
	}
	if _, ok := b.reserve(classBackground); ok {
		t.Fatal("bucket refilled beyond the elapsed time")
	}
}

// Пока пользователь ждёт ответа, фон не имеет права забирать токены: иначе
// открытая карточка игры стоит в очереди за дозагрузкой каталога.
func TestBackgroundYieldsWhileAUserRequestIsPending(t *testing.T) {
	clock := newClock()
	b := newBudget(clock.Now)
	b.enter()
	defer b.leave()

	wait, ok := b.reserve(classBackground)
	if ok {
		t.Fatal("background took a token ahead of a pending user request")
	}
	if wait != backYield {
		t.Fatalf("wait = %v, want the yield step %v", wait, backYield)
	}
	if _, ok := b.reserve(classUser); !ok {
		t.Fatal("user request refused while the budget is full")
	}
}

func TestPenaltyHonoursRetryAfterAndCapsIt(t *testing.T) {
	cases := []struct {
		name       string
		retryAfter time.Duration
		want       time.Duration
	}{
		{"подсказка сервера", 30 * time.Second, 30 * time.Second},
		{"выше потолка", time.Hour, maxPause},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			clock := newClock()
			b := newBudget(clock.Now)
			if got := b.penalize(tc.retryAfter); got != tc.want {
				t.Fatalf("pause = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestPenaltyWithoutAHintPausesForOneTokenInterval(t *testing.T) {
	clock := newClock()
	b := newBudget(clock.Now)
	pause := b.penalize(0)
	if pause <= 0 || pause > time.Duration(1/minRate*float64(time.Second)) {
		t.Fatalf("pause = %v, want one interval of the shrunk rate", pause)
	}
}

func TestPenaltyNeverShortensARunningPause(t *testing.T) {
	clock := newClock()
	b := newBudget(clock.Now)
	long := b.penalize(time.Minute)
	short := b.penalize(time.Second)
	if short < long {
		t.Fatalf("pause shrank from %v to %v", long, short)
	}
}

func TestBackgroundGivesUpOnALongPauseWithoutBlocking(t *testing.T) {
	clock := newClock()
	b := newBudget(clock.Now)
	b.penalize(maxPause)

	err := b.acquire(context.Background(), classBackground)
	if !errors.Is(err, errBudgetBusy) {
		t.Fatalf("err = %v, want errBudgetBusy", err)
	}
	if len(b.back) != 0 {
		t.Fatal("background slot leaked after the budget refused the call")
	}
}

// Отказ пользователю обязан нести число: «повтор через N» вместо спиннера на
// весь таймаут и пустого списка кандидатов.
func TestUserGetsTheRemainingPauseInsteadOfATimeout(t *testing.T) {
	clock := newClock()
	b := newBudget(clock.Now)
	b.penalize(maxPause)

	err := b.acquire(context.Background(), classUser)
	var limit *RateLimitError
	if !errors.As(err, &limit) {
		t.Fatalf("err = %v, want a RateLimitError", err)
	}
	if limit.RetryAfter <= userMaxWait {
		t.Fatalf("retry after = %v, want the remaining pause", limit.RetryAfter)
	}
	if len(b.user) != 0 {
		t.Fatal("user slot leaked after the budget refused the call")
	}
}

func TestShortPauseIsAbsorbedForTheUser(t *testing.T) {
	b := newBudget(nil)
	b.penalize(minPauseStep)

	if err := b.acquire(context.Background(), classUser); err != nil {
		t.Fatalf("user call refused over a %v pause: %v", minPauseStep, err)
	}
	b.release(classUser)
}

func TestCancelledContextReleasesTheSlot(t *testing.T) {
	clock := newClock()
	b := newBudget(clock.Now)
	b.penalize(time.Second)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := b.acquire(ctx, classUser); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if len(b.user) != 0 {
		t.Fatal("user slot leaked on cancellation")
	}
}

type blockingImages struct {
	inFlight chan struct{}
	release  chan struct{}
	body     []byte
}

func (h *blockingImages) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	h.inFlight <- struct{}{}
	<-h.release
	w.Header().Set("Content-Type", "image/png")
	if _, err := w.Write(h.body); err != nil {
		return
	}
}

// Картинки идут на CDN провайдера и не должны занимать слоты запросов к API:
// на общей очереди девять зависших скриншотов блокировали поиск из карточки.
func TestStuckImageDownloadsDoNotBlockUserRequests(t *testing.T) {
	const games = 3
	handler := &blockingImages{
		inFlight: make(chan struct{}, games),
		release:  make(chan struct{}),
		body:     pngBytes(t, 264, 374),
	}
	images := httptest.NewServer(handler)
	defer images.Close()

	meta := map[string]GameMetadata{}
	for i := range games {
		meta[strconv.Itoa(i)] = GameMetadata{
			ProviderID: strconv.Itoa(i),
			Title:      "Prey",
			Cover:      &ImageRef{URL: images.URL + "/cover.png", Width: 264, Height: 374},
		}
	}
	provider := &fakeProvider{meta: meta, candidates: []Candidate{{ProviderID: "0", Title: "Prey"}}}
	svc, cat, _ := newTestService(t, provider)

	var wg sync.WaitGroup
	for i := range games {
		game := addGame(t, cat, catalog.Game{Title: "Prey " + strconv.Itoa(i)})
		wg.Add(1)
		go func(id, providerID string) {
			defer wg.Done()
			if _, err := svc.ApplyMatch(id, providerID); err != nil {
				t.Errorf("apply match: %v", err)
			}
		}(game.ID, strconv.Itoa(i))
	}
	for range games {
		<-handler.inFlight
	}

	done := make(chan error, 1)
	go func() { _, err := svc.SearchCandidates("prey"); done <- err }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("search while images are stuck: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("user search starved by in-flight image downloads")
	}

	close(handler.release)
	wg.Wait()
}

func TestBudgetCountsOnlyProviderCalls(t *testing.T) {
	srv := imageServer(t)
	provider := &fakeProvider{meta: map[string]GameMetadata{"2657": fullMetadata(srv.URL)}}
	svc, cat, _ := newTestService(t, provider)
	game := addGame(t, cat, catalog.Game{Title: "Prey"})

	if _, err := svc.ApplyMatch(game.ID, "2657"); err != nil {
		t.Fatalf("apply match: %v", err)
	}
	if got := len(svc.screenshots(game.ID)); got != maxScreenshots {
		t.Fatalf("screenshots = %d, want %d", got, maxScreenshots)
	}

	_, spent, _, _ := svc.budget.stats()
	if spent != 1 {
		t.Fatalf("budget spent = %d, want the single provider Get and no image downloads", spent)
	}
}

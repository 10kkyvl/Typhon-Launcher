package metadata

import (
	"context"
	"errors"
	"sync"
	"time"
)

const (
	startRate    = 2.0
	minRate      = 0.5
	maxRate      = 8.0
	burst        = 8.0
	rateGrowth   = 0.05
	rateShrink   = 0.5
	userSlots    = 4
	backSlots    = 2
	imageSlots   = 6
	userMaxWait  = 5 * time.Second
	backMaxWait  = 20 * time.Second
	backYield    = 200 * time.Millisecond
	maxPause     = 2 * time.Minute
	minPauseStep = 250 * time.Millisecond
)

var errBudgetBusy = errors.New("бюджет запросов к провайдеру исчерпан")

type callClass int

const (
	classUser callClass = iota
	classBackground
)

type budget struct {
	mu          sync.Mutex
	rate        float64
	tokens      float64
	last        time.Time
	pausedUntil time.Time
	userPending int
	penalties   int
	spent       int
	now         func() time.Time

	user chan struct{}
	back chan struct{}
}

func newBudget(now func() time.Time) *budget {
	if now == nil {
		now = time.Now
	}
	return &budget{
		rate:   startRate,
		tokens: burst,
		last:   now(),
		now:    now,
		user:   make(chan struct{}, userSlots),
		back:   make(chan struct{}, backSlots),
	}
}

func (b *budget) slots(class callClass) chan struct{} {
	if class == classUser {
		return b.user
	}
	return b.back
}

func (b *budget) acquire(ctx context.Context, class callClass) error {
	slots := b.slots(class)
	select {
	case slots <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	if err := b.pace(ctx, class); err != nil {
		<-slots
		return err
	}
	return nil
}

func (b *budget) release(class callClass) {
	<-b.slots(class)
}

func (b *budget) pace(ctx context.Context, class callClass) error {
	if class == classUser {
		b.enter()
		defer b.leave()
	}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		wait, ok := b.reserve(class)
		if ok {
			return nil
		}
		if class == classBackground && wait > backMaxWait {
			return errBudgetBusy
		}
		// Спиннер на всё время паузы хуже честного «повтор через N»: длинную
		// паузу пользователь должен увидеть, а не досидеть до таймаута.
		if class == classUser {
			deadline, has := ctx.Deadline()
			if wait > userMaxWait || (has && b.now().Add(wait).After(deadline)) {
				return &RateLimitError{RetryAfter: wait}
			}
		}
		if !b.rest(ctx, wait) {
			return ctx.Err()
		}
	}
}

func (b *budget) reserve(class callClass) (time.Duration, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()

	now := b.now()
	if wait := b.pausedUntil.Sub(now); wait > 0 {
		return wait, false
	}
	// Фон уступает очередь пользователю: пока есть ожидающий запрос от UI,
	// фоновая дозагрузка обложек токенов не берёт.
	if class == classBackground && b.userPending > 0 {
		return backYield, false
	}

	b.advanceLocked(now)
	if b.tokens < 1 {
		return time.Duration((1-b.tokens)/b.rate*float64(time.Second)) + minPauseStep, false
	}
	b.tokens--
	b.spent++
	return 0, true
}

func (b *budget) advanceLocked(now time.Time) {
	elapsed := now.Sub(b.last)
	if elapsed <= 0 {
		return
	}
	b.tokens = min(b.tokens+elapsed.Seconds()*b.rate, burst)
	b.last = now
}

func (b *budget) rest(ctx context.Context, wait time.Duration) bool {
	if wait < minPauseStep {
		wait = minPauseStep
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-ctx.Done():
		return false
	}
}

func (b *budget) enter() {
	b.mu.Lock()
	b.userPending++
	b.mu.Unlock()
}

func (b *budget) leave() {
	b.mu.Lock()
	b.userPending--
	b.mu.Unlock()
}

// penalize сбрасывает темп по факту 429: сервер — единственный источник правды
// о своём бюджете, поэтому Retry-After важнее локальной оценки.
func (b *budget) penalize(retryAfter time.Duration) time.Duration {
	b.mu.Lock()
	defer b.mu.Unlock()

	now := b.now()
	b.rate = max(b.rate*rateShrink, minRate)
	b.tokens = 0
	b.last = now
	b.penalties++

	wait := retryAfter
	if wait <= 0 {
		wait = time.Duration(1 / b.rate * float64(time.Second))
	}
	wait = min(wait, maxPause)
	if until := now.Add(wait); until.After(b.pausedUntil) {
		b.pausedUntil = until
	}
	return b.pausedUntil.Sub(now)
}

func (b *budget) reward() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.rate = min(b.rate+rateGrowth, maxRate)
}

func (b *budget) stats() (rate float64, spent, penalties int, pause time.Duration) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.rate, b.spent, b.penalties, max(b.pausedUntil.Sub(b.now()), 0)
}

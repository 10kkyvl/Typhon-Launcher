package tray

import (
	"context"
	"sync"
	"time"
)

type Timer interface {
	Stop() bool
}

// Coalescer folds a burst of triggers into one run per delay. A game start
// fires game:started and library:updated back to back, and rebuilding a
// native menu or a Jump List for each of them is both slow and visible.
type Coalescer struct {
	delay time.Duration
	run   func()
	after func(time.Duration, func()) Timer

	mu      sync.Mutex
	timer   Timer
	armed   bool
	stopped bool
	wg      sync.WaitGroup
	runMu   sync.Mutex
}

func NewCoalescer(ctx context.Context, delay time.Duration, run func()) *Coalescer {
	return newCoalescer(ctx, delay, run, func(d time.Duration, f func()) Timer {
		return time.AfterFunc(d, f)
	})
}

func newCoalescer(ctx context.Context, delay time.Duration, run func(), after func(time.Duration, func()) Timer) *Coalescer {
	c := &Coalescer{delay: delay, run: run, after: after}
	context.AfterFunc(ctx, c.Close)
	return c
}

func (c *Coalescer) Trigger() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.stopped || c.armed {
		return
	}
	c.armed = true
	c.wg.Add(1)
	c.timer = c.after(c.delay, c.fire)
}

func (c *Coalescer) fire() {
	defer c.wg.Done()
	c.mu.Lock()
	c.armed = false
	c.timer = nil
	stopped := c.stopped
	c.mu.Unlock()
	if stopped {
		return
	}
	c.runMu.Lock()
	defer c.runMu.Unlock()
	c.run()
}

// Close disarms a pending run; a run that already started is left to finish,
// and Wait covers it.
func (c *Coalescer) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.stopped = true
	if c.timer != nil && c.timer.Stop() {
		c.wg.Done()
		c.armed = false
	}
	c.timer = nil
}

func (c *Coalescer) Wait() {
	c.wg.Wait()
}

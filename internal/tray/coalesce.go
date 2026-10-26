package tray

import (
	"context"
	"sync"
	"time"
)

type Timer interface {
	Stop() bool
}

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

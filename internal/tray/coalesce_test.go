package tray

import (
	"context"
	"testing"
	"time"
)

type fakeTimer struct {
	f        func()
	delay    time.Duration
	canceled bool
	fired    bool
}

func (t *fakeTimer) Stop() bool {
	if t.fired || t.canceled {
		return false
	}
	t.canceled = true
	return true
}

type fakeClock struct{ timers []*fakeTimer }

func (c *fakeClock) after(d time.Duration, f func()) Timer {
	t := &fakeTimer{f: f, delay: d}
	c.timers = append(c.timers, t)
	return t
}

func (c *fakeClock) fire(i int) {
	timer := c.timers[i]
	timer.fired = true
	timer.f()
}

func TestCoalescerFoldsABurstIntoOneRun(t *testing.T) {
	clock := &fakeClock{}
	runs := 0
	c := newCoalescer(t.Context(), 500*time.Millisecond, func() { runs++ }, clock.after)

	c.Trigger()
	c.Trigger()
	c.Trigger()
	if len(clock.timers) != 1 || clock.timers[0].delay != 500*time.Millisecond {
		t.Fatalf("timers = %+v, want one timer of 500ms", clock.timers)
	}
	if runs != 0 {
		t.Fatal("run happened before the timer fired")
	}
	clock.fire(0)
	if runs != 1 {
		t.Fatalf("runs = %d, want 1", runs)
	}

	c.Trigger()
	if len(clock.timers) != 2 {
		t.Fatalf("a trigger after the run must arm a new timer, timers = %d", len(clock.timers))
	}
	clock.fire(1)
	if runs != 2 {
		t.Fatalf("runs = %d, want 2", runs)
	}
	c.Wait()
}

func TestCoalescerTriggerDuringRunSchedulesAnother(t *testing.T) {
	clock := &fakeClock{}
	var c *Coalescer
	runs := 0
	c = newCoalescer(t.Context(), time.Second, func() {
		runs++
		if runs == 1 {
			c.Trigger()
		}
	}, clock.after)

	c.Trigger()
	clock.fire(0)
	if len(clock.timers) != 2 {
		t.Fatalf("an event that arrives mid-run must not be lost, timers = %d", len(clock.timers))
	}
	clock.fire(1)
	if runs != 2 {
		t.Fatalf("runs = %d, want 2", runs)
	}
	c.Wait()
}

func TestCoalescerCancelDisarmsThePendingRun(t *testing.T) {
	clock := &fakeClock{}
	runs := 0
	ctx, cancel := context.WithCancel(t.Context())
	c := newCoalescer(ctx, time.Second, func() { runs++ }, clock.after)

	c.Trigger()
	cancel()
	c.Wait()
	if !clock.timers[0].canceled {
		t.Fatal("the pending timer was not stopped on cancel")
	}
	if runs != 0 {
		t.Fatalf("runs = %d, want 0", runs)
	}

	c.Trigger()
	if len(clock.timers) != 1 {
		t.Fatal("a trigger after cancel armed a timer")
	}
}

func TestCoalescerSkipsARunWhoseTimerAlreadyFiredBeforeClose(t *testing.T) {
	clock := &fakeClock{}
	runs := 0
	c := newCoalescer(t.Context(), time.Second, func() { runs++ }, clock.after)

	c.Trigger()
	clock.timers[0].fired = true
	c.Close()
	clock.timers[0].f()
	if runs != 0 {
		t.Fatalf("runs = %d, want 0: the timer goroutine must notice the close", runs)
	}
	c.Wait()
}

func TestCoalescerRealTimerRuns(t *testing.T) {
	done := make(chan struct{})
	c := NewCoalescer(t.Context(), time.Millisecond, func() { close(done) })
	c.Trigger()
	<-done
	c.Wait()
}

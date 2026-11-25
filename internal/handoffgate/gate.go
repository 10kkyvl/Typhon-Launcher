// Package handoffgate holds back the background work of a launcher that was
// started as the successor in a seamless self-update.
//
// The successor builds its services and renders the update screen while the
// outgoing launcher is still alive, because the outgoing window may only close
// once the new one is on screen. Two launchers must not both run the loops
// that write shared state (sync, downloads, presence, the library watcher), so
// those loops call Wait first: it returns at once in a normal start and blocks
// in a handoff start until the outgoing process is gone and Open is called.
package handoffgate

import (
	"context"
	"log/slog"
	"sync"
)

var (
	pending sync.WaitGroup
	mu      sync.Mutex
	armed   bool
	open    = make(chan struct{})
	// held is every service whose startup the gate deferred and that has not
	// finished it yet, by the name the service gave Startup.
	held = map[string]*heldStart{}
)

type heldStart struct {
	running bool          // the deferred startup has begun
	stopped bool          // the service is shutting down; the startup must not begin
	done    chan struct{} // closed when a begun startup has returned
}

// Arm closes the gate. Call it once, before the services start.
func Arm() {
	mu.Lock()
	defer mu.Unlock()
	if armed {
		return
	}
	armed = true
	open = make(chan struct{})
}

// Open lets every waiter through. It is idempotent.
func Open() {
	mu.Lock()
	defer mu.Unlock()
	if !armed {
		return
	}
	armed = false
	close(open)
	// A service already shut down is done with: nothing calls it any more,
	// and its name may start afresh.
	for name, h := range held {
		if h.stopped {
			delete(held, name)
		}
	}
}

// Armed reports whether the gate is currently closed.
func Armed() bool {
	mu.Lock()
	defer mu.Unlock()
	return armed
}

// Wait blocks until the gate is open and reports true; it reports false when
// ctx ends first, in which case the caller must not start its work.
func Wait(ctx context.Context) bool {
	mu.Lock()
	if !armed {
		mu.Unlock()
		return ctx.Err() == nil
	}
	ch := open
	mu.Unlock()
	select {
	case <-ch:
		return ctx.Err() == nil
	case <-ctx.Done():
		return false
	}
}

// Startup runs a service's startup now in a normal launcher. In a successor it
// returns at once and runs start when the gate opens, so the service reads and
// writes its state only after the old launcher has gone. A failure there can
// no longer stop the application from starting and is logged.
//
// name identifies the service to Stopping and HeldBack and must be unique.
func Startup(ctx context.Context, name string, start func() error) error {
	mu.Lock()
	if !armed {
		delete(held, name)
		mu.Unlock()
		return start()
	}
	h := &heldStart{done: make(chan struct{})}
	held[name] = h
	pending.Add(1)
	ch := open
	mu.Unlock()
	go func() {
		defer pending.Done()
		select {
		case <-ch:
		case <-ctx.Done():
			return
		}
		mu.Lock()
		if h.stopped || ctx.Err() != nil {
			mu.Unlock()
			return
		}
		h.running = true
		mu.Unlock()
		err := start()
		mu.Lock()
		if held[name] == h {
			delete(held, name)
		}
		mu.Unlock()
		close(h.done)
		if err != nil {
			slog.Error("deferred service startup failed", "service", name, "error", err)
		}
	}()
	return nil
}

// Stopping is the first call of a service's shutdown. It reports whether the
// service's startup ran (or is running, in which case it waits for it): only
// then has the service loaded state it may save or work it must stop. A
// startup still held back is cancelled for good, and the shutdown must do
// nothing: the service's lists were never read, so saving them would replace
// the user's state with empty ones.
func Stopping(name string) bool {
	mu.Lock()
	h, ok := held[name]
	if !ok {
		mu.Unlock()
		return true
	}
	if !h.running {
		h.stopped = true
		mu.Unlock()
		slog.Info("service shut down before its held-back startup ran, nothing to stop", "service", name)
		return false
	}
	mu.Unlock()
	<-h.done
	return true
}

// HeldBack reports whether the named service's startup was deferred by the
// gate and has not begun: the service has not read its state, and a call
// that would change it must refuse.
func HeldBack(name string) bool {
	mu.Lock()
	defer mu.Unlock()
	h, ok := held[name]
	return ok && !h.running
}

// Settled waits until every startup held back by the gate has finished. Call
// it after Open, before the interface is told the launcher is ready.
func Settled(ctx context.Context) error {
	done := make(chan struct{})
	go func() {
		pending.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

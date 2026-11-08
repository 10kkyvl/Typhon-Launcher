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
)

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
func Startup(ctx context.Context, name string, start func() error) error {
	mu.Lock()
	if !armed {
		mu.Unlock()
		return start()
	}
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
		if err := start(); err != nil {
			slog.Error("deferred service startup failed", "service", name, "error", err)
		}
	}()
	return nil
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

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
	"sync"
)

var (
	mu    sync.Mutex
	armed bool
	open  = make(chan struct{})
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

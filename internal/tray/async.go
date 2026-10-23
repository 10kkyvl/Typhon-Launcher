package tray

import "sync"

// Async runs menu click handlers off the UI thread: launching a game can wait
// on the library lock for as long as the process takes to start, and a menu
// callback that blocks freezes the whole tray. Every goroutine is counted, and
// after Close nothing new starts.
type Async struct {
	mu     sync.Mutex
	closed bool
	wg     sync.WaitGroup
}

func (a *Async) Go(f func()) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed {
		return false
	}
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		f()
	}()
	return true
}

// Close refuses new work and waits for the running one.
func (a *Async) Close() {
	a.mu.Lock()
	a.closed = true
	a.mu.Unlock()
	a.wg.Wait()
}

package tray

import "sync"

// Click handlers run off the UI thread: PlayGame holds the library lock while
// the process starts, and a blocked menu callback freezes the tray.
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

func (a *Async) Close() {
	a.mu.Lock()
	a.closed = true
	a.mu.Unlock()
	a.wg.Wait()
}

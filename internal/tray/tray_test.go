package tray

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
)

type fakeTray struct {
	destroyed int32
}

func (t *fakeTray) Destroy() {
	atomic.AddInt32(&t.destroyed, 1)
}

func (t *fakeTray) destroyCount() int {
	return int(atomic.LoadInt32(&t.destroyed))
}

type fakeWindow struct {
	mu     sync.Mutex
	events []string
}

func (w *fakeWindow) Show() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.events = append(w.events, "show")
}

func (w *fakeWindow) Hide() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.events = append(w.events, "hide")
}

func (w *fakeWindow) Focus() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.events = append(w.events, "focus")
}

func (w *fakeWindow) snapshot() []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	out := make([]string, len(w.events))
	copy(out, w.events)
	return out
}

func (w *fakeWindow) count(name string) int {
	w.mu.Lock()
	defer w.mu.Unlock()
	n := 0
	for _, e := range w.events {
		if e == name {
			n++
		}
	}
	return n
}

func newCountingCreate() (func() (Tray, error), *int32, *atomic.Value) {
	var calls int32
	var last atomic.Value
	create := func() (Tray, error) {
		atomic.AddInt32(&calls, 1)
		t := &fakeTray{}
		last.Store(t)
		return t, nil
	}
	return create, &calls, &last
}

func newCountingQuit() (func(), func() int) {
	var calls int32
	quit := func() {
		atomic.AddInt32(&calls, 1)
	}
	return quit, func() int { return int(atomic.LoadInt32(&calls)) }
}

func TestNewValidatesDependencies(t *testing.T) {
	validWindow := &fakeWindow{}
	validCreate := func() (Tray, error) { return &fakeTray{}, nil }
	validQuit := func() {}

	tests := []struct {
		name    string
		window  Window
		create  func() (Tray, error)
		quit    func()
		wantErr bool
	}{
		{"nil window", nil, validCreate, validQuit, true},
		{"nil create", validWindow, nil, validQuit, true},
		{"nil quit", validWindow, validCreate, nil, true},
		{"all valid", validWindow, validCreate, validQuit, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := New(tt.window, tt.create, tt.quit)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("New() error = nil, want error")
				}
				if c != nil {
					t.Fatalf("New() controller = %v, want nil on error", c)
				}
				return
			}
			if err != nil {
				t.Fatalf("New() unexpected error: %v", err)
			}
			if c == nil {
				t.Fatalf("New() controller = nil, want non-nil")
			}
		})
	}
}

func TestApplyEnableTwiceCreatesOnce(t *testing.T) {
	create, calls, _ := newCountingCreate()
	quit, _ := newCountingQuit()
	c, err := New(&fakeWindow{}, create, quit)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	if err := c.Apply(true); err != nil {
		t.Fatalf("first Apply(true) error: %v", err)
	}
	if err := c.Apply(true); err != nil {
		t.Fatalf("second Apply(true) error: %v", err)
	}

	if got := atomic.LoadInt32(calls); got != 1 {
		t.Fatalf("create called %d times, want 1", got)
	}
}

func TestApplyDisableAfterEnableDestroysOnce(t *testing.T) {
	create, _, last := newCountingCreate()
	quit, _ := newCountingQuit()
	c, err := New(&fakeWindow{}, create, quit)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	if err := c.Apply(true); err != nil {
		t.Fatalf("Apply(true) error: %v", err)
	}
	tr, ok := last.Load().(*fakeTray)
	if !ok {
		t.Fatalf("create did not record a tray")
	}

	if err := c.Apply(false); err != nil {
		t.Fatalf("Apply(false) error: %v", err)
	}
	if got := tr.destroyCount(); got != 1 {
		t.Fatalf("Destroy called %d times, want 1", got)
	}

	if err := c.Apply(false); err != nil {
		t.Fatalf("second Apply(false) error: %v", err)
	}
	if got := tr.destroyCount(); got != 1 {
		t.Fatalf("Destroy called %d times after redundant disable, want 1", got)
	}
}

func TestApplyDisableThenEnableRecreates(t *testing.T) {
	create, calls, last := newCountingCreate()
	quit, _ := newCountingQuit()
	c, err := New(&fakeWindow{}, create, quit)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	if err := c.Apply(false); err != nil {
		t.Fatalf("Apply(false) on empty controller error: %v", err)
	}
	if got := atomic.LoadInt32(calls); got != 0 {
		t.Fatalf("create called %d times, want 0", got)
	}

	if err := c.Apply(true); err != nil {
		t.Fatalf("Apply(true) error: %v", err)
	}
	first, ok := last.Load().(*fakeTray)
	if !ok {
		t.Fatalf("create did not record the first tray")
	}

	if err := c.Apply(false); err != nil {
		t.Fatalf("Apply(false) error: %v", err)
	}

	if err := c.Apply(true); err != nil {
		t.Fatalf("Apply(true) after disable error: %v", err)
	}
	second, ok := last.Load().(*fakeTray)
	if !ok {
		t.Fatalf("create did not record the second tray")
	}

	if got := atomic.LoadInt32(calls); got != 2 {
		t.Fatalf("create called %d times, want 2", got)
	}
	if first == second {
		t.Fatalf("expected a new tray instance after re-enable")
	}
}

var errCreateFailed = errors.New("boom")

func TestApplyCreateErrorLeavesNoTrayAndCloseIsNotCancelled(t *testing.T) {
	create := func() (Tray, error) { return nil, errCreateFailed }
	quit, quitCalls := newCountingQuit()
	window := &fakeWindow{}
	c, err := New(window, create, quit)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	err = c.Apply(true)
	if err == nil {
		t.Fatalf("Apply(true) error = nil, want wrapped create error")
	}
	if !errors.Is(err, errCreateFailed) {
		t.Fatalf("Apply(true) error = %v, want wrapping errCreateFailed", err)
	}

	closing := c.CloseRequested()
	if closing {
		t.Fatalf("CloseRequested() = true, want false when no tray exists")
	}
	if got := quitCalls(); got != 0 {
		t.Fatalf("quit called %d times, want 0: the close event itself ends the app", got)
	}
	if got := window.count("hide"); got != 0 {
		t.Fatalf("Hide called %d times, want 0 (window must not be lost)", got)
	}
}

func TestApplyCreateReturnsNilTrayIsError(t *testing.T) {
	create := func() (Tray, error) { return nil, nil }
	quit, quitCalls := newCountingQuit()
	window := &fakeWindow{}
	c, err := New(window, create, quit)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	err = c.Apply(true)
	if err == nil {
		t.Fatalf("Apply(true) error = nil, want error for nil tray")
	}

	if c.CloseRequested() {
		t.Fatalf("CloseRequested() = true, want false since tray was never set")
	}
	if got := quitCalls(); got != 0 {
		t.Fatalf("quit called %d times, want 0: the close event itself ends the app", got)
	}
}

func TestCloseRequestedWithTrayHidesWindow(t *testing.T) {
	create, _, _ := newCountingCreate()
	quit, quitCalls := newCountingQuit()
	window := &fakeWindow{}
	c, err := New(window, create, quit)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	if err := c.Apply(true); err != nil {
		t.Fatalf("Apply(true) error: %v", err)
	}

	closing := c.CloseRequested()
	if !closing {
		t.Fatalf("CloseRequested() = false, want true when tray exists")
	}
	if got := window.count("hide"); got != 1 {
		t.Fatalf("Hide called %d times, want 1", got)
	}
	if got := quitCalls(); got != 0 {
		t.Fatalf("quit called %d times, want 0", got)
	}
}

func TestCloseRequestedWithoutTrayIsNotCancelled(t *testing.T) {
	create, _, _ := newCountingCreate()
	quit, quitCalls := newCountingQuit()
	window := &fakeWindow{}
	c, err := New(window, create, quit)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	closing := c.CloseRequested()
	if closing {
		t.Fatalf("CloseRequested() = true, want false without tray")
	}
	if got := window.count("hide"); got != 0 {
		t.Fatalf("Hide called %d times, want 0", got)
	}
	if got := quitCalls(); got != 0 {
		t.Fatalf("quit called %d times, want 0: the close event itself ends the app", got)
	}
}

func TestOpenShowsThenFocuses(t *testing.T) {
	create, _, _ := newCountingCreate()
	quit, _ := newCountingQuit()
	window := &fakeWindow{}
	c, err := New(window, create, quit)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	c.Open()

	got := window.snapshot()
	want := []string{"show", "focus"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("Open() events = %v, want %v", got, want)
	}
}

func TestQuitDestroysTrayBeforeQuitting(t *testing.T) {
	create, _, last := newCountingCreate()
	quit, quitCalls := newCountingQuit()
	window := &fakeWindow{}
	c, err := New(window, create, quit)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	if err := c.Apply(true); err != nil {
		t.Fatalf("Apply(true) error: %v", err)
	}
	tr, ok := last.Load().(*fakeTray)
	if !ok {
		t.Fatalf("create did not record a tray")
	}

	c.Quit()

	if got := quitCalls(); got != 1 {
		t.Fatalf("quit called %d times, want 1", got)
	}
	if got := tr.destroyCount(); got != 1 {
		t.Fatalf("Destroy called %d times, want 1", got)
	}
	if c.CloseRequested() {
		t.Fatalf("CloseRequested() = true after Quit, want false: shutdown must not be cancelled")
	}
	if got := window.count("hide"); got != 0 {
		t.Fatalf("Hide called %d times after Quit, want 0", got)
	}
}

func TestConcurrentApplyAndCloseRequestedIsRaceFree(t *testing.T) {
	create, _, _ := newCountingCreate()
	quit, _ := newCountingQuit()
	window := &fakeWindow{}
	c, err := New(window, create, quit)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	const iterations = 200
	var wg sync.WaitGroup
	wg.Add(4)

	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			if err := c.Apply(true); err != nil {
				t.Errorf("Apply(true) error: %v", err)
			}
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			if err := c.Apply(false); err != nil {
				t.Errorf("Apply(false) error: %v", err)
			}
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			c.CloseRequested()
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			c.Open()
		}
	}()

	wg.Wait()
}

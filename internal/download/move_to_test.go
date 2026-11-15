package download

import (
	"errors"
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	"typhon/internal/uierr"
)

// mixedQueue lays out queued downloads with others between them, so that a
// move has to keep the paused, running and finished ones where they are.
func mixedQueue(t *testing.T) *Manager {
	t.Helper()
	m := newTestManager(t, 1)
	for _, item := range []struct {
		id     string
		status Status
	}{
		{"q1", StatusQueued}, {"P", StatusPaused}, {"q2", StatusQueued}, {"D", StatusDownloading},
		{"q3", StatusQueued}, {"q4", StatusQueued}, {"C", StatusCompleted},
	} {
		m.addTestItem(item.id, item.status)
	}
	return m
}

func persistedOrder(t *testing.T, m *Manager) []string {
	t.Helper()
	var ids []string
	for _, r := range persistedRecords(t, m) {
		ids = append(ids, r.ID)
	}
	return ids
}

func (l *emitLog) reorders() []ReorderedEvent {
	l.mu.Lock()
	defer l.mu.Unlock()
	var out []ReorderedEvent
	for _, e := range l.events {
		if ev, ok := e.data.(ReorderedEvent); ok && e.name == eventReordered {
			out = append(out, ev)
		}
	}
	return out
}

func TestMoveToPlacesAQueuedDownloadAmongTheQueuedOnes(t *testing.T) {
	cases := []struct {
		name     string
		id       string
		position int
		want     []string
	}{
		{"to the top", "q3", 0, []string{"q3", "P", "q1", "D", "q2", "q4", "C"}},
		{"to the end", "q1", 3, []string{"q2", "P", "q3", "D", "q4", "q1", "C"}},
		{"into the middle", "q4", 1, []string{"q1", "P", "q4", "D", "q2", "q3", "C"}},
		{"before the top clamps to the top", "q2", -5, []string{"q2", "P", "q1", "D", "q3", "q4", "C"}},
		{"past the end clamps to the end", "q2", 99, []string{"q1", "P", "q3", "D", "q4", "q2", "C"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := mixedQueue(t)
			log := recordEmits(t)

			if err := m.MoveTo(c.id, c.position); err != nil {
				t.Fatalf("MoveTo(%s, %d): %v", c.id, c.position, err)
			}

			if got := m.order(); !slices.Equal(got, c.want) {
				t.Fatalf("order = %v, want %v", got, c.want)
			}
			if got := persistedOrder(t, m); !slices.Equal(got, c.want) {
				t.Fatalf("persisted order = %v, want %v", got, c.want)
			}
			reorders := log.reorders()
			if len(reorders) != 1 || !slices.Equal(reorders[0].IDs, c.want) {
				t.Fatalf("reorder events = %+v, want one carrying %v", reorders, c.want)
			}
			if n := log.count(eventUpdated); n != 0 {
				t.Fatalf("download:updated emitted %d times; a move changes no download, only the order", n)
			}
		})
	}
}

func TestMoveToItsOwnPlaceWritesAndSaysNothing(t *testing.T) {
	m := mixedQueue(t)
	before := m.order()
	log := recordEmits(t)
	// A write would fail now; a move that changes nothing must not attempt one.
	breakStore(t, m)

	if err := m.MoveTo("q2", 1); err != nil {
		t.Fatalf("MoveTo to the place it holds: %v", err)
	}
	if got := m.order(); !slices.Equal(got, before) {
		t.Fatalf("order = %v, want %v", got, before)
	}
	if st := m.degradedStatus(); st.Degraded {
		t.Fatalf("degraded = %+v: a move to the same place wrote the file", st)
	}
	if n := log.mark(); n != 0 {
		t.Fatalf("%d events emitted for a move that changed nothing", n)
	}
}

func TestMoveToRefusesWhatIsNotQueued(t *testing.T) {
	cases := []struct {
		id   string
		code string
	}{
		{"nope", "download.not_found"},
		{"P", "download.unavailable"},
		{"D", "download.unavailable"},
		{"C", "download.unavailable"},
	}
	for _, c := range cases {
		t.Run(c.id, func(t *testing.T) {
			m := mixedQueue(t)
			before := m.order()
			log := recordEmits(t)

			err := m.MoveTo(c.id, 0)

			if got := uierr.Code(err); got != c.code {
				t.Fatalf("MoveTo(%s) = %v, code %q, want %q", c.id, err, got, c.code)
			}
			if got := m.order(); !slices.Equal(got, before) {
				t.Fatalf("order = %v, want %v", got, before)
			}
			if n := log.mark(); n != 0 {
				t.Fatalf("%d events emitted for a refused move", n)
			}
		})
	}
}

func TestMoveToRollsBackOnPersistFailure(t *testing.T) {
	m := mixedQueue(t)
	before := m.order()
	log := recordEmits(t)
	breakStore(t, m)

	err := m.MoveTo("q4", 0)

	if err == nil {
		t.Fatal("MoveTo must return the persist failure")
	}
	if got := uierr.Code(err); got != "download.save_failed" {
		t.Fatalf("code = %q (%v), want download.save_failed", got, err)
	}
	if got := m.order(); !slices.Equal(got, before) {
		t.Fatalf("order = %v, want it rolled back to %v", got, before)
	}
	if n := log.count(eventReordered) + log.count(eventUpdated); n != 0 {
		t.Fatalf("%d order or download events emitted for a move that was not saved", n)
	}
	if st := m.degradedStatus(); !st.Degraded {
		t.Fatalf("degraded = %+v, want the failed write surfaced", st)
	}
}

func TestMoveToOrderSurvivesARestart(t *testing.T) {
	m := mixedQueue(t)
	if err := m.MoveTo("q4", 0); err != nil {
		t.Fatal(err)
	}
	want := m.order()

	next := reopen(t, m)

	var got []string
	for _, d := range next.List() {
		got = append(got, d.ID)
	}
	if !slices.Equal(got, want) {
		t.Fatalf("order after restart = %v, want %v", got, want)
	}
}

func TestMoveToAgreesWithMoveUpAndMoveDown(t *testing.T) {
	up := mixedQueue(t)
	if err := up.MoveUp("q3"); err != nil {
		t.Fatal(err)
	}
	to := mixedQueue(t)
	if err := to.MoveTo("q3", 1); err != nil {
		t.Fatal(err)
	}
	if a, b := up.order(), to.order(); !slices.Equal(a, b) {
		t.Fatalf("MoveUp gives %v, MoveTo one place up gives %v", a, b)
	}

	down := mixedQueue(t)
	if err := down.MoveDown("q1"); err != nil {
		t.Fatal(err)
	}
	to = mixedQueue(t)
	if err := to.MoveTo("q1", 1); err != nil {
		t.Fatal(err)
	}
	if a, b := down.order(), to.order(); !slices.Equal(a, b) {
		t.Fatalf("MoveDown gives %v, MoveTo one place down gives %v", a, b)
	}
}

func (l *emitLog) names() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]string, 0, len(l.events))
	for _, e := range l.events {
		out = append(out, e.name)
	}
	return out
}

func listedOrder(m *Manager) []string {
	var ids []string
	for _, d := range m.List() {
		ids = append(ids, d.ID)
	}
	return ids
}

// A step up or down tells the window the whole new order the way MoveTo does,
// besides the two downloads it swapped, so the window does not have to read the
// list again to learn it.
func TestAStepSaysTheNewOrder(t *testing.T) {
	cases := []struct {
		name string
		step func(m *Manager) error
	}{
		{"up", func(m *Manager) error { return m.MoveUp("q3") }},
		{"down", func(m *Manager) error { return m.MoveDown("q1") }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := mixedQueue(t)
			log := recordEmits(t)

			if err := c.step(m); err != nil {
				t.Fatal(err)
			}

			reorders := log.reorders()
			if want := listedOrder(m); len(reorders) != 1 || !slices.Equal(reorders[0].IDs, want) {
				t.Fatalf("reorder events = %+v, want one carrying %v", reorders, want)
			}
			want := []string{eventUpdated, eventUpdated, eventReordered}
			if got := log.names(); !slices.Equal(got, want) {
				t.Fatalf("events = %v, want %v", got, want)
			}
		})
	}
}

func TestAStepThatDoesNotMoveSaysNothing(t *testing.T) {
	cases := []struct {
		name string
		step func(m *Manager) error
		code string
	}{
		{"the first queued up", func(m *Manager) error { return m.MoveUp("q1") }, ""},
		{"the last queued down", func(m *Manager) error { return m.MoveDown("q4") }, ""},
		{"a paused one", func(m *Manager) error { return m.MoveUp("P") }, "download.unavailable"},
		{"an unknown one", func(m *Manager) error { return m.MoveDown("nope") }, "download.not_found"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := mixedQueue(t)
			before := m.order()
			log := recordEmits(t)

			err := c.step(m)

			if got := uierr.Code(err); got != c.code || (c.code == "" && err != nil) {
				t.Fatalf("step = %v, code %q, want %q", err, got, c.code)
			}
			if got := m.order(); !slices.Equal(got, before) {
				t.Fatalf("order = %v, want %v", got, before)
			}
			if got := log.names(); len(got) != 0 {
				t.Fatalf("events %v for a step that moved nothing", got)
			}
		})
	}
}

func TestAStepThatIsNotSavedSaysNoOrder(t *testing.T) {
	m := mixedQueue(t)
	before := m.order()
	log := recordEmits(t)
	breakStore(t, m)

	err := m.MoveDown("q1")

	if got := uierr.Code(err); got != "download.save_failed" {
		t.Fatalf("code = %q (%v), want download.save_failed", got, err)
	}
	if got := m.order(); !slices.Equal(got, before) {
		t.Fatalf("order = %v, want it rolled back to %v", got, before)
	}
	if n := log.count(eventReordered) + log.count(eventUpdated); n != 0 {
		t.Fatalf("%d order or download events emitted for a step that was not saved", n)
	}
}

func TestConcurrentMovesAndPausesKeepEveryDownloadOnce(t *testing.T) {
	m := newTestManager(t, 1)
	var ids []string
	for i := range 8 {
		id := fmt.Sprintf("q%d", i)
		ids = append(ids, id)
		m.addTestItem(id, StatusQueued)
	}
	log := recordEmits(t)

	var wg sync.WaitGroup
	errs := make(chan error, 256)
	for w := range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range 30 {
				id := ids[(w*7+i)%len(ids)]
				err := m.MoveTo(id, (i*3+w)%len(ids))
				if err != nil && !errors.Is(err, errUnavailable) {
					errs <- fmt.Errorf("MoveTo(%s): %w", id, err)
				}
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for _, id := range []string{"q1", "q4", "q6"} {
			if err := m.Pause(id); err != nil {
				errs <- fmt.Errorf("Pause(%s): %w", id, err)
			}
		}
	}()
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("concurrent moves and pauses did not finish: deadlock")
	}
	close(errs)
	for err := range errs {
		t.Error(err)
	}

	got := m.order()
	sorted := slices.Clone(got)
	slices.Sort(sorted)
	if !slices.Equal(sorted, ids) {
		t.Fatalf("order %v lost or doubled a download", got)
	}
	if persisted := persistedOrder(t, m); !slices.Equal(persisted, got) {
		t.Fatalf("persisted order %v, memory order %v", persisted, got)
	}
	assertStatuses(t, m, map[string]Status{"q1": StatusPaused, "q4": StatusPaused, "q6": StatusPaused})
	if reorders := log.reorders(); len(reorders) > 0 && !slices.Equal(reorders[len(reorders)-1].IDs, got) {
		t.Fatalf("last reorder event %v, final order %v", reorders[len(reorders)-1].IDs, got)
	}
}

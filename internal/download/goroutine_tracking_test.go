package download

import (
	"context"
	"testing"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
)

func TestSpawnTrackedLockedSkipsWhenClosing(t *testing.T) {
	m := newTestManager(t, 1)
	m.mu.Lock()
	m.closing = true
	started := m.spawnTrackedLocked(func() {})
	m.mu.Unlock()
	if started {
		t.Fatal("spawnTrackedLocked started a goroutine while the manager is closing")
	}
}

// TestSpawnTrackedLockedIsWaitedOn proves the wg.Add/Done pairing is
// correct: wg.Wait must not return before the tracked function has actually
// run to completion. Since Done is deferred until after fn returns, Wait
// returning at all already implies fn ran; the channel check below is a
// belt-and-braces confirmation of that ordering rather than a race in
// itself.
func TestSpawnTrackedLockedIsWaitedOn(t *testing.T) {
	m := newTestManager(t, 1)
	ran := make(chan struct{})
	m.mu.Lock()
	started := m.spawnTrackedLocked(func() { close(ran) })
	m.mu.Unlock()
	if !started {
		t.Fatal("spawnTrackedLocked refused to start")
	}

	waitReturned := make(chan struct{})
	go func() {
		m.wg.Wait()
		close(waitReturned)
	}()

	select {
	case <-waitReturned:
	case <-time.After(2 * time.Second):
		t.Fatal("wg.Wait never returned for a tracked goroutine")
	}
	select {
	case <-ran:
	default:
		t.Fatal("wg.Wait returned before the tracked function ran")
	}
}

// TestServiceShutdownWaitsForPendingTeardown reproduces the danger bug #3
// describes: a DeleteData/Cancel/Remove teardown that is still waiting on an
// in-flight job's job.done when ServiceShutdown is called. Before the fix
// that teardown ran in a bare `go func(){...}()` untracked by m.wg, so
// ServiceShutdown could return (and the process could exit) while it was
// still mid-flight — on Windows leaving a locked download directory behind.
// The fake job here stands in for a slow verify/restore: its own cancel is a
// no-op, and it only finishes once this test releases gate, proving
// ServiceShutdown really waits rather than merely happening to finish first.
func TestServiceShutdownWaitsForPendingTeardown(t *testing.T) {
	m := mustManagerAt(t, t.TempDir())
	if err := m.ServiceStartup(context.Background(), application.ServiceOptions{}); err != nil {
		t.Fatalf("startup: %v", err)
	}
	m.addTestItem("a", StatusCompleted)

	gate := make(chan struct{})
	job := &jobState{cancel: func() {}, done: make(chan struct{})}
	m.mu.Lock()
	m.jobs["a"] = job
	m.mu.Unlock()
	go func() {
		<-gate
		close(job.done)
	}()

	if err := m.DeleteData("a"); err != nil {
		t.Fatalf("DeleteData: %v", err)
	}

	shutdownDone := make(chan error, 1)
	go func() {
		shutdownDone <- m.ServiceShutdown()
	}()

	select {
	case err := <-shutdownDone:
		t.Fatalf("ServiceShutdown returned (%v) before the pending teardown's job finished", err)
	case <-time.After(100 * time.Millisecond):
	}

	close(gate)

	select {
	case err := <-shutdownDone:
		if err != nil {
			t.Fatalf("ServiceShutdown: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("ServiceShutdown never returned after the pending teardown finished (deadlock or lost wg tracking)")
	}
}

// TestDeleteDataSkipsTeardownWhileClosing shows the other half of the fix:
// once the manager is closing, DeleteData/discard must not start a new
// teardown goroutine at all. A bare, untracked `go func(){...}()` would
// still drop the engine here almost immediately (there is no job to block
// on), which is exactly what this checks does not happen: m.wg.Wait
// returning quickly is not by itself proof of anything, since an untracked
// goroutine is invisible to it too.
func TestDeleteDataSkipsTeardownWhileClosing(t *testing.T) {
	m := newTestManager(t, 1)
	m.addTestItem("a", StatusCompleted)
	eng := &fakeTorrent{size: 100}
	m.mu.Lock()
	m.engines["a"] = eng
	m.closing = true
	m.mu.Unlock()

	if err := m.DeleteData("a"); err != nil {
		t.Fatalf("DeleteData: %v", err)
	}
	if got := len(m.List()); got != 0 {
		t.Fatalf("items tracked = %d, want 0 (the record is still removed)", got)
	}

	waitReturned := make(chan struct{})
	go func() {
		m.wg.Wait()
		close(waitReturned)
	}()
	select {
	case <-waitReturned:
	case <-time.After(2 * time.Second):
		t.Fatal("wg.Wait blocked, meaning some other tracked goroutine leaked into this test")
	}

	// Give a wrongly-started bare goroutine a fair chance to run before
	// declaring it absent.
	<-time.After(50 * time.Millisecond)
	if eng.wasDropped() {
		t.Fatal("engine was dropped by a teardown that must not have started while closing")
	}
}

// gatedEngine задерживает drop до открытия ворот, чтобы отличить учтённую в
// m.wg горутину от голой: у голой wg.Wait вернулся бы, не дожидаясь дропа.
type gatedEngine struct {
	*fakeTorrent
	gate chan struct{}
}

func (g *gatedEngine) drop() {
	<-g.gate
	g.fakeTorrent.drop()
}

// TestDetachEngineDropIsTracked закрывает ту же дыру, что и teardown из
// DeleteData: detachEngineLocked отпускает движок завершённой загрузки, когда
// сидирование выключено, и его drop закрывает хранилище торрента. Голой
// горутиной он мог выполниться уже после ServiceShutdown — на Windows это
// каталог, заблокированный для переустановки.
func TestDetachEngineDropIsTracked(t *testing.T) {
	m := newTestManager(t, 1)
	eng := &gatedEngine{fakeTorrent: &fakeTorrent{size: 100}, gate: make(chan struct{})}

	m.mu.Lock()
	m.engines["a"] = eng
	m.detachEngineLocked("a", eng)
	m.mu.Unlock()

	waitReturned := make(chan struct{})
	go func() {
		m.wg.Wait()
		close(waitReturned)
	}()
	select {
	case <-waitReturned:
		t.Fatal("wg.Wait returned while the engine drop was still in flight")
	case <-time.After(100 * time.Millisecond):
	}

	close(eng.gate)
	select {
	case <-waitReturned:
	case <-time.After(2 * time.Second):
		t.Fatal("wg.Wait never returned after the engine drop finished")
	}
	if !eng.wasDropped() {
		t.Fatal("engine was never dropped")
	}
}

// TestDetachEngineSkipsDropWhileClosing — вторая половина: при закрытии
// менеджера новая горутина не стартует вовсе, движок закроется через
// cl.close() в ServiceShutdown.
func TestDetachEngineSkipsDropWhileClosing(t *testing.T) {
	m := newTestManager(t, 1)
	eng := &fakeTorrent{size: 100}

	m.mu.Lock()
	m.engines["a"] = eng
	m.closing = true
	m.detachEngineLocked("a", eng)
	m.mu.Unlock()

	waitReturned := make(chan struct{})
	go func() {
		m.wg.Wait()
		close(waitReturned)
	}()
	select {
	case <-waitReturned:
	case <-time.After(2 * time.Second):
		t.Fatal("wg.Wait blocked, meaning some other tracked goroutine leaked into this test")
	}

	<-time.After(50 * time.Millisecond)
	if eng.wasDropped() {
		t.Fatal("engine was dropped by a goroutine that must not have started while closing")
	}
}

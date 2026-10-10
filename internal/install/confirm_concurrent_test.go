package install

import (
	"path/filepath"
	"sync"
	"testing"
	"time"

	"typhon/internal/library"
)

// barrierLibrary держит каждую регистрацию, пока не соберутся все want вызовов
// или не истечёт окно: при ошибке в сервисе все подтверждения доходят сюда
// одновременно при любом расписании потоков, а честный путь платит одно окно.
type barrierLibrary struct {
	*fakeRegistrar
	want   int
	window time.Duration
	mu     sync.Mutex
	in     int
	open   chan struct{}
}

func (b *barrierLibrary) RegisterInstalled(g library.InstalledGame) (library.Game, error) {
	b.mu.Lock()
	b.in++
	if b.in == b.want {
		close(b.open)
	}
	b.mu.Unlock()
	select {
	case <-b.open:
	case <-time.After(b.window):
	}
	return b.fakeRegistrar.RegisterInstalled(g)
}

func TestConcurrentConfirmsCompleteExactlyOnce(t *testing.T) {
	r := newRig(t)
	const callers = 8
	lib := &barrierLibrary{fakeRegistrar: r.reg, want: callers, window: 400 * time.Millisecond, open: make(chan struct{})}
	r.lib = lib
	r.s.library = lib
	installed := interactiveSource(t, r)
	r.setRunner(newRgRunner(t, rgStep{act: func(runSpec) { mkFile(t, filepath.Join(installed, "MyGame.exe"), 512<<10) }}))
	item, err := r.s.Start("d1", StartOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got := r.settle(item.ID); got.Status != StatusWaitingForUser {
		t.Fatalf("status = %s (%q)", got.Status, got.Error)
	}
	finished := r.finished()
	exe := filepath.Join(installed, "MyGame.exe")

	errs := callTogether(callers, func(int) error { return r.s.ConfirmExecutable(item.ID, exe) })

	for _, err := range errs {
		if err != nil && errCode(err) != "install.unavailable" {
			t.Fatalf("a refused confirm = %v, want install.unavailable", err)
		}
	}
	r.shutdown()
	accepted := countNil(errs)
	notified := len(finished.drain())
	registered := len(r.reg.registered())
	if accepted != 1 || registered != 1 || notified != 1 {
		t.Fatalf("%d of %d concurrent confirms accepted, %d games registered, %d finish notifications; want 1, 1 and 1",
			accepted, len(errs), registered, notified)
	}
	if got := r.get(item.ID); got.Status != StatusCompleted || got.Executable != exe {
		t.Fatalf("record after the confirms: status %s executable %q", got.Status, got.Executable)
	}
}

func TestConfirmClaimsTheRecordBeforeCompleting(t *testing.T) {
	r := newRig(t)
	installed := interactiveSource(t, r)
	r.setRunner(newRgRunner(t, rgStep{act: func(runSpec) { mkFile(t, filepath.Join(installed, "MyGame.exe"), 512<<10) }}))
	item, err := r.s.Start("d1", StartOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got := r.settle(item.ID); got.Status != StatusWaitingForUser {
		t.Fatalf("status = %s (%q)", got.Status, got.Error)
	}
	exe := filepath.Join(installed, "MyGame.exe")
	unblock := r.blockStore()

	wantPersistError(t, r.s.ConfirmExecutable(item.ID, exe))
	unblock()

	got := r.get(item.ID)
	if got.Status != StatusWaitingForUser || got.GameID != "" {
		t.Fatalf("after a confirm that could not be saved: status %s game %q, want the record back to waiting for the user", got.Status, got.GameID)
	}
	if err := r.s.ConfirmExecutable(item.ID, exe); err != nil {
		t.Fatalf("a repeated confirm after the failed one = %v, want it accepted", err)
	}
	if got := r.get(item.ID); got.Status != StatusCompleted {
		t.Fatalf("status after the repeated confirm = %s (%q)", got.Status, got.Error)
	}
}

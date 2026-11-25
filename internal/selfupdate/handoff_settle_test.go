package selfupdate

import (
	"errors"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"typhon/internal/handoffgate"
	"typhon/internal/windowhandoff"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// settleCall is what the successor's step to an ordinary app saw when it ran.
type settleCall struct {
	focus, oldGone, revealed bool
}

// successorRig is a successor launcher whose old process lives while oldAlive
// is set; the reveal and the settle step are recorded.
type successorRig struct {
	svc      *Service
	oldAlive atomic.Bool
	revealed atomic.Bool
	mu       sync.Mutex
	settles  []settleCall
}

func newSuccessorRig(t *testing.T, focused bool) *successorRig {
	t.Helper()
	silenceSeams(t)
	cfg := t.TempDir()
	ready, err := HandoffReadyPath(cfg, "abc")
	if err != nil {
		t.Fatal(err)
	}
	r := &successorRig{svc: &Service{dir: cfg, store: mustStore(t, cfg), currentVersion: "1.5.0", ctx: t.Context()}}
	r.oldAlive.Store(true)
	r.svc.SetHandoff(HandoffSpec{Nonce: "abc", ParentPID: 4321, ReadyPath: ready, Version: "1.5.0", Geometry: windowhandoff.Geometry{Width: 1, Height: 1, Focused: focused}})
	r.svc.SetHandoffWindow(&application.WebviewWindow{})

	prevReveal, prevSettle, prevAlive, prevPoll := revealWindow, settleWindow, parentAlive, parentPollInterval
	revealWindow = func(*application.WebviewWindow, bool) error { r.revealed.Store(true); return nil }
	settleWindow = func(_ *application.WebviewWindow, focus bool) error {
		r.mu.Lock()
		r.settles = append(r.settles, settleCall{focus: focus, oldGone: !r.oldAlive.Load(), revealed: r.revealed.Load()})
		r.mu.Unlock()
		return nil
	}
	parentAlive = func(int) (bool, error) { return r.oldAlive.Load(), nil }
	parentPollInterval = 5 * time.Millisecond
	t.Cleanup(func() {
		revealWindow, settleWindow, parentAlive, parentPollInterval = prevReveal, prevSettle, prevAlive, prevPoll
	})
	handoffgate.Arm()
	t.Cleanup(handoffgate.Open)
	return r
}

func (r *successorRig) calls() []settleCall {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]settleCall(nil), r.settles...)
}

func (r *successorRig) complete(t *testing.T) error {
	t.Helper()
	return r.svc.CompleteHandoff(t.Context(), func() error { return nil })
}

func TestSuccessorBecomesAnOrdinaryAppOnlyOnceTheOldLauncherIsGone(t *testing.T) {
	r := newSuccessorRig(t, true)
	if err := r.svc.HandoffRendered(); err != nil {
		t.Fatalf("HandoffRendered() = %v", err)
	}
	if got := r.calls(); len(got) != 0 {
		t.Fatalf("settled %v while the old launcher still runs", got)
	}

	done := make(chan error, 1)
	go func() { done <- r.complete(t) }()
	r.oldAlive.Store(false)
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("CompleteHandoff() = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("CompleteHandoff did not return after the old launcher exited")
	}

	want := []settleCall{{focus: true, oldGone: true, revealed: true}}
	if got := r.calls(); len(got) != 1 || got[0] != want[0] {
		t.Fatalf("settle calls = %+v, want %+v", got, want)
	}
}

// The old launcher died before the successor's window was revealed (no ready
// marker was ever seen) and the page never reports its first frame: once the
// old process is known gone the successor shows its window and becomes an
// ordinary app by itself; the page's late report changes nothing.
func TestSuccessorWhoseOldLauncherDiedBeforeTheRevealShowsItself(t *testing.T) {
	r := newSuccessorRig(t, true)
	reveals := 0
	revealWindow = func(*application.WebviewWindow, bool) error {
		reveals++
		r.revealed.Store(true)
		return nil
	}
	r.oldAlive.Store(false)
	if err := r.complete(t); err != nil {
		t.Fatalf("CompleteHandoff() = %v", err)
	}
	if reveals != 1 {
		t.Fatalf("revealed %d times without the page, want once", reveals)
	}
	want := settleCall{focus: true, oldGone: true, revealed: true}
	if got := r.calls(); len(got) != 1 || got[0] != want {
		t.Fatalf("settle calls = %+v, want one %+v", got, want)
	}

	if err := r.svc.HandoffRendered(); err != nil {
		t.Fatalf("the page's late HandoffRendered() = %v", err)
	}
	if reveals != 1 || len(r.calls()) != 1 {
		t.Fatalf("the late report revealed %d times and settled %d times, want no more", reveals, len(r.calls()))
	}
}

func TestSuccessorTakesOverEvenWhenItCannotBecomeAnOrdinaryApp(t *testing.T) {
	r := newSuccessorRig(t, true)
	settled := 0
	settleWindow = func(*application.WebviewWindow, bool) error {
		settled++
		return errors.New("the regular activation policy was refused")
	}
	if err := r.svc.HandoffRendered(); err != nil {
		t.Fatalf("HandoffRendered() = %v", err)
	}
	r.oldAlive.Store(false)
	if err := r.complete(t); err != nil {
		t.Fatalf("CompleteHandoff() = %v, want the hand-off finished", err)
	}
	if settled != 1 || r.svc.HandoffInfo().Active || handoffgate.Armed() {
		t.Fatalf("settle tried %d times, active=%v, gate armed=%v; want one try and a finished hand-off", settled, r.svc.HandoffInfo().Active, handoffgate.Armed())
	}
}

func TestOldLauncherYieldsTheActivationToItsSuccessorBeforeItQuits(t *testing.T) {
	for _, focused := range []bool{true, false} {
		t.Run(map[bool]string{true: "active", false: "inactive"}[focused], func(t *testing.T) {
			r := newHandoffRig(t, func(spec HandoffSpec, _ *fakeSuccessor) {
				if err := os.WriteFile(spec.ReadyPath, []byte(spec.Nonce), 0o600); err != nil { //nolint:forbidigo // test double for the successor's marker
					t.Errorf("write the marker: %v", err)
				}
			})
			r.svc.SetHandoffWindow(&application.WebviewWindow{})
			prevCapture := captureGeometry
			captureGeometry = func(*application.WebviewWindow) windowhandoff.Geometry {
				return windowhandoff.Geometry{Width: 800, Height: 600, Focused: focused}
			}
			t.Cleanup(func() { captureGeometry = prevCapture })
			quit := make(chan struct{})
			quitApplication = func() { close(quit) }
			var yields []int
			yieldForeground = func(pid int) error {
				select {
				case <-quit:
					t.Error("the activation was yielded after the quit")
				default:
				}
				yields = append(yields, pid)
				return nil
			}

			r.svc.runHandoff(t.Context(), r.plan, 1)
			select {
			case <-quit:
			case <-time.After(5 * time.Second):
				t.Fatal("the old launcher did not quit")
			}

			want := 0
			if focused {
				want = 1
			}
			if len(yields) != want || (want == 1 && yields[0] != r.proc.Pid()) {
				t.Errorf("yields = %v, want %d to pid %d", yields, want, r.proc.Pid())
			}
		})
	}
}

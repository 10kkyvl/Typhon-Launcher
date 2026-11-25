package windowhandoff

import (
	"errors"
	"testing"
)

// fakeWindow is a Windows window as revealStyled sees it: SetLayeredWindowAttributes
// fails on a window that is not layered, as the real call does.
type fakeWindow struct {
	style       uint32
	alpha       int
	transitions bool // disabled
	framed      int
}

func (w *fakeWindow) exStyle() uint32         { return w.style }
func (w *fakeWindow) setExStyle(style uint32) { w.style = style }
func (w *fakeWindow) setAlpha(alpha byte) error {
	if w.style&exLayered == 0 {
		return errors.New("SetLayeredWindowAttributes on a window that is not layered")
	}
	w.alpha = int(alpha)
	return nil
}
func (w *fakeWindow) setTransitions(disabled bool) error { w.transitions = disabled; return nil }
func (w *fakeWindow) frameChanged() error                { w.framed++; return nil }

const (
	testHidden = exLayered | 0x20 | 0x08000000 // layered, transparent, no-activate
	testOwn    = 0x100                         // a bit the window had of its own
)

// The successor's Preparer failed and the window was shown with a plain Show:
// it is visible and not layered, so there is nothing to reveal, and failing
// here would keep the ready marker from being written and roll the update
// back 20 s later.
func TestRevealSucceedsOnAWindowShownPlainly(t *testing.T) {
	w := &fakeWindow{style: testOwn}
	if err := revealStyled(w, 0, false, testHidden); err != nil {
		t.Fatalf("revealStyled() on a plainly shown window = %v, want nil", err)
	}
	if w.style != testOwn || w.transitions {
		t.Fatalf("window after reveal = %+v, want its style kept and transitions on", *w)
	}
}

func TestRevealRestoresAWindowPaintHiddenMadeInvisible(t *testing.T) {
	w := &fakeWindow{style: testOwn | testHidden, alpha: 0, transitions: true}
	if err := revealStyled(w, testOwn, true, testHidden); err != nil {
		t.Fatalf("revealStyled() = %v", err)
	}
	if w.alpha != 255 || w.style != testOwn || w.transitions || w.framed != 1 {
		t.Fatalf("window after reveal = %+v, want opaque, its own style back, transitions on, frame redrawn", *w)
	}
}

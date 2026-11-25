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

func TestRevealRestoresAWindowPaintHiddenMadeInvisible(t *testing.T) {
	w := &fakeWindow{style: testOwn | testHidden, alpha: 0, transitions: true}
	if err := revealStyled(w, testOwn, true, testHidden); err != nil {
		t.Fatalf("revealStyled() = %v", err)
	}
	if w.alpha != 255 || w.style != testOwn || w.transitions || w.framed != 1 {
		t.Fatalf("window after reveal = %+v, want opaque, its own style back, transitions on, frame redrawn", *w)
	}
}

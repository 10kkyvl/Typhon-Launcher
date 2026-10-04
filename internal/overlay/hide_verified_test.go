package overlay

import (
	"strings"
	"testing"
)

type stickyWindow struct {
	hwnd    uintptr
	ignored int
	visible bool
	hides   int
}

func (w *stickyWindow) handle() uintptr { return w.hwnd }

func (w *stickyWindow) hide() {
	w.hides++
	if w.ignored > 0 {
		w.ignored--
		return
	}
	w.visible = false
}

func (w *stickyWindow) isVisible() bool { return w.visible }

func TestHideVerified(t *testing.T) {
	cases := []struct {
		name      string
		visible   bool
		ignored   int
		want      bool
		wantHides int
		wantWarns int
	}{
		{"already hidden by the first call", true, 0, true, 1, 0},
		{"was not on screen", false, 0, true, 1, 0},
		{"hides on the retry", true, 1, true, 2, 1},
		{"stays on screen through the retry", true, never, false, 2, 2},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out := captureLog(t)
			w := &stickyWindow{hwnd: 777, ignored: c.ignored, visible: c.visible}

			if got := hideVerified(w, "test window"); got != c.want {
				t.Fatalf("hideVerified = %v, want %v", got, c.want)
			}
			if w.hides != c.wantHides {
				t.Fatalf("hide called %d times, want %d", w.hides, c.wantHides)
			}
			text := out.String()
			if got := strings.Count(text, "level=WARN"); got != c.wantWarns {
				t.Fatalf("%d warnings, want %d: %s", got, c.wantWarns, text)
			}
			if c.wantWarns > 0 && !strings.Contains(text, "hwnd=777") {
				t.Fatalf("warning lacks the window handle: %s", text)
			}
		})
	}
}

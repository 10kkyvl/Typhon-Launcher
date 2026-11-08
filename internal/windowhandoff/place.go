package windowhandoff

import "github.com/wailsapp/wails/v3/pkg/application"

// Units. Wails reports Position and Size in logical (DPI-independent) units on
// every platform, in one global space whose origin is the top-left corner of
// the primary screen with Y pointing down:
//   - macOS: points; windowGetPosition flips Cocoa's bottom-left origin.
//   - Windows: DIPs. windowsWebviewWindow.position/size run the physical
//     GetWindowRect through Screen.PhysicalToDipRect, which accounts for the
//     per-monitor DPI of every screen in the layout.
//
// So Geometry needs no DPI conversion; ScreenID and Scale are carried only to
// detect that the monitor layout changed between the two processes.

// minVisible is the smallest part of the window, per axis, that must stay on a
// screen for the saved position to be considered reachable by the user.
const minVisible = 120

type rect struct{ X, Y, W, H int }

// screenInfo is the part of application.Screen the placement logic needs.
type screenInfo struct {
	ID        string
	Work      rect
	IsPrimary bool
}

func screensFromApp(app *application.App) []screenInfo {
	if app == nil || app.Screen == nil {
		return nil
	}
	all := app.Screen.GetAll()
	out := make([]screenInfo, 0, len(all))
	for _, s := range all {
		if s == nil {
			continue
		}
		out = append(out, screenInfo{
			ID:        s.ID,
			Work:      rect{s.WorkArea.X, s.WorkArea.Y, s.WorkArea.Width, s.WorkArea.Height},
			IsPrimary: s.IsPrimary,
		})
	}
	return out
}

func overlap(a, b rect) (w, h int) {
	left, right := max(a.X, b.X), min(a.X+a.W, b.X+b.W)
	top, bottom := max(a.Y, b.Y), min(a.Y+a.H, b.Y+b.H)
	return max(0, right-left), max(0, bottom-top)
}

func reachable(r rect, s screenInfo) bool {
	w, h := overlap(r, s.Work)
	return w >= min(minVisible, r.W) && h >= min(minVisible, r.H)
}

func centred(r rect, s screenInfo) rect {
	w, h := min(r.W, s.Work.W), min(r.H, s.Work.H)
	return rect{s.Work.X + (s.Work.W-w)/2, s.Work.Y + (s.Work.H-h)/2, w, h}
}

func primaryOf(screens []screenInfo) (screenInfo, bool) {
	for _, s := range screens {
		if s.IsPrimary {
			return s, true
		}
	}
	if len(screens) > 0 {
		return screens[0], true
	}
	return screenInfo{}, false
}

// place decides where the successor window goes. The saved rectangle is kept
// when its screen still exists and the window is reachable on it; otherwise
// the window is centred on the primary screen (or on its own screen when that
// one exists but the rectangle fell off it).
func place(g Geometry, screens []screenInfo) rect {
	r := rect{g.X, g.Y, g.Width, g.Height}
	primary, ok := primaryOf(screens)
	if !ok {
		return r
	}
	if g.ScreenID != "" {
		for _, s := range screens {
			if s.ID != g.ScreenID {
				continue
			}
			if reachable(r, s) {
				return r
			}
			return centred(r, s)
		}
		return centred(r, primary)
	}
	for _, s := range screens {
		if reachable(r, s) {
			return r
		}
	}
	return centred(r, primary)
}

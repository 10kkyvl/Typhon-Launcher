package windowhandoff

import (
	"errors"
	"reflect"
	"testing"
)

func TestEncodeDecodeRoundTrip(t *testing.T) {
	in := Geometry{X: -1200, Y: 40, Width: 1280, Height: 800, Maximised: true, Fullscreen: false, ScreenID: "2", Scale: 1.5, Focused: true}
	arg, err := Encode(in)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	for _, c := range arg {
		if c == '=' || c == '+' || c == '/' || c == ' ' || c == '"' {
			t.Fatalf("argument %q is not command-line safe", arg)
		}
	}
	var out Geometry
	if err := Decode(arg, &out); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if !reflect.DeepEqual(in, out) {
		t.Fatalf("round trip: got %+v, want %+v", out, in)
	}
}

func TestDecodeRejectsGarbage(t *testing.T) {
	var g Geometry
	if err := Decode("!!not base64!!", &g); err == nil {
		t.Fatal("expected a base64 error")
	}
	arg, err := Encode("just a string")
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if err := Decode(arg, &g); err == nil {
		t.Fatal("expected a JSON type error")
	}
}

func TestValid(t *testing.T) {
	if (Geometry{}).Valid() {
		t.Fatal("zero geometry must be invalid")
	}
	if !(Geometry{Width: 1, Height: 1}).Valid() {
		t.Fatal("sized geometry must be valid")
	}
	if !errors.Is(ErrNoGeometry, ErrNoGeometry) {
		t.Fatal("sentinel")
	}
}

var (
	primary   = screenInfo{ID: "1", Work: rect{0, 25, 1440, 875}, IsPrimary: true}
	secondary = screenInfo{ID: "2", Work: rect{1440, -200, 1920, 1080}}
)

func TestPlaceKeepsReachableRect(t *testing.T) {
	g := Geometry{X: 1600, Y: 0, Width: 1000, Height: 700, ScreenID: "2"}
	got := place(g, []screenInfo{primary, secondary})
	if want := (rect{1600, 0, 1000, 700}); got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestPlaceMissingScreenCentresOnPrimary(t *testing.T) {
	g := Geometry{X: 1600, Y: 0, Width: 1000, Height: 700, ScreenID: "2"}
	got := place(g, []screenInfo{primary})
	if want := (rect{220, 25 + 87, 1000, 700}); got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestPlaceOffScreenOnKnownScreenCentresOnThatScreen(t *testing.T) {
	g := Geometry{X: 5000, Y: 5000, Width: 1000, Height: 700, ScreenID: "2"}
	got := place(g, []screenInfo{primary, secondary})
	if want := (rect{1440 + 460, -200 + 190, 1000, 700}); got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestPlaceWithoutScreenID(t *testing.T) {
	visible := Geometry{X: 100, Y: 100, Width: 800, Height: 600}
	if got := place(visible, []screenInfo{primary}); got != (rect{100, 100, 800, 600}) {
		t.Fatalf("visible rect moved: %+v", got)
	}
	off := Geometry{X: -3000, Y: 100, Width: 800, Height: 600}
	if got := place(off, []screenInfo{primary}); got != (rect{320, 25 + 137, 800, 600}) {
		t.Fatalf("off-screen rect not centred: %+v", got)
	}
}

func TestPlaceShrinksToWorkArea(t *testing.T) {
	g := Geometry{X: 9000, Y: 0, Width: 3000, Height: 2000, ScreenID: "gone"}
	got := place(g, []screenInfo{primary})
	if want := (rect{0, 25, 1440, 875}); got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestPlaceBarelyVisibleIsNotReachable(t *testing.T) {
	g := Geometry{X: 1430, Y: 100, Width: 800, Height: 600, ScreenID: "1"}
	got := place(g, []screenInfo{primary})
	if got.X != 320 {
		t.Fatalf("10px sliver should be centred, got %+v", got)
	}
}

func TestPlaceNoScreens(t *testing.T) {
	g := Geometry{X: 5, Y: 6, Width: 7, Height: 8, ScreenID: "1"}
	if got := place(g, nil); got != (rect{5, 6, 7, 8}) {
		t.Fatalf("got %+v", got)
	}
}

func TestPlacePrimaryFallbackWhenNoneMarked(t *testing.T) {
	s := screenInfo{ID: "9", Work: rect{0, 0, 1000, 800}}
	got := place(Geometry{X: -5000, Y: 0, Width: 400, Height: 400}, []screenInfo{s})
	if got != (rect{300, 200, 400, 400}) {
		t.Fatalf("got %+v", got)
	}
}

func TestPreparerWaitsForTheNativeWindowAndPreparesOnce(t *testing.T) {
	native := false
	calls := 0
	p := &Preparer{
		ready:   func() bool { return native },
		prepare: func() error { calls++; return nil },
	}

	if prepared, err := p.Try(); prepared || err != nil || calls != 0 {
		t.Fatalf("Try before the native window exists = %v, %v with %d prepare calls; want it left for later", prepared, err, calls)
	}

	native = true
	if prepared, err := p.Try(); !prepared || err != nil || calls != 1 {
		t.Fatalf("Try once the native window exists = %v, %v with %d prepare calls; want it prepared", prepared, err, calls)
	}
	if prepared, err := p.Try(); prepared || err != nil || calls != 1 {
		t.Fatalf("second Try = %v, %v with %d prepare calls; want nothing done twice", prepared, err, calls)
	}
}

func TestPreparerHandsBackWhyPreparingFailed(t *testing.T) {
	p := &Preparer{
		ready:   func() bool { return true },
		prepare: func() error { return ErrNoGeometry },
	}
	prepared, err := p.Try()
	if !prepared || !errors.Is(err, ErrNoGeometry) {
		t.Fatalf("Try = %v, %v; want the attempt reported with ErrNoGeometry", prepared, err)
	}
}

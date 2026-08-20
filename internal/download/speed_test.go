package download

import (
	"testing"
	"time"
)

func TestRateWindowAverages(t *testing.T) {
	base := time.Now()
	w := newRateWindow(5 * time.Second)
	for i := 0; i <= 4; i++ {
		w.add(base.Add(time.Duration(i)*time.Second), int64(i)*1000)
	}
	if got := w.rate(); got != 1000 {
		t.Fatalf("rate = %d, want 1000", got)
	}
}

func TestRateWindowNeedsTwoSamples(t *testing.T) {
	w := newRateWindow(5 * time.Second)
	if got := w.rate(); got != 0 {
		t.Fatalf("empty rate = %d, want 0", got)
	}
	w.add(time.Now(), 5000)
	if got := w.rate(); got != 0 {
		t.Fatalf("single sample rate = %d, want 0", got)
	}
}

func TestRateWindowEvictsOldSamples(t *testing.T) {
	base := time.Now()
	w := newRateWindow(5 * time.Second)
	for i := 0; i <= 11; i++ {
		w.add(base.Add(time.Duration(i)*time.Second), int64(i)*1000)
	}
	if len(w.samples) != 6 {
		t.Fatalf("samples = %d, want 6", len(w.samples))
	}
	if !w.samples[0].at.Equal(base.Add(6 * time.Second)) {
		t.Fatalf("oldest sample = %v, want %v", w.samples[0].at, base.Add(6*time.Second))
	}
	if got := w.rate(); got != 1000 {
		t.Fatalf("rate = %d, want 1000", got)
	}
}

func TestRateWindowIgnoresDecreases(t *testing.T) {
	base := time.Now()
	w := newRateWindow(5 * time.Second)
	w.add(base, 9000)
	w.add(base.Add(time.Second), 4000)
	if got := w.rate(); got != 0 {
		t.Fatalf("rate = %d, want 0", got)
	}
}

func TestRateWindowReplacesSameTimestamp(t *testing.T) {
	base := time.Now()
	w := newRateWindow(5 * time.Second)
	w.add(base, 0)
	w.add(base, 500)
	if len(w.samples) != 1 {
		t.Fatalf("samples = %d, want 1", len(w.samples))
	}
	if w.samples[0].bytes != 500 {
		t.Fatalf("bytes = %d, want 500", w.samples[0].bytes)
	}
}

func TestETASeconds(t *testing.T) {
	cases := []struct {
		remaining, rate, want int64
	}{
		{1000, 100, 10},
		{1000, 0, -1},
		{0, 100, 0},
		{0, 0, 0},
		{150, 100, 1},
	}
	for _, c := range cases {
		if got := etaSeconds(c.remaining, c.rate); got != c.want {
			t.Fatalf("etaSeconds(%d, %d) = %d, want %d", c.remaining, c.rate, got, c.want)
		}
	}
}

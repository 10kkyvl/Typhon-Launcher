package download

import "time"

const defaultRateSpan = 5 * time.Second

type rateSample struct {
	at    time.Time
	bytes int64
}

type rateWindow struct {
	span    time.Duration
	samples []rateSample
}

func newRateWindow(span time.Duration) *rateWindow {
	if span <= 0 {
		span = defaultRateSpan
	}
	return &rateWindow{span: span}
}

func (w *rateWindow) add(at time.Time, total int64) {
	if n := len(w.samples); n > 0 && !at.After(w.samples[n-1].at) {
		w.samples[n-1].bytes = total
		return
	}
	w.samples = append(w.samples, rateSample{at: at, bytes: total})

	cutoff := at.Add(-w.span)
	drop := 0
	for drop < len(w.samples)-2 && w.samples[drop].at.Before(cutoff) {
		drop++
	}
	if drop > 0 {
		w.samples = append(w.samples[:0], w.samples[drop:]...)
	}
}

func (w *rateWindow) rate() int64 {
	if len(w.samples) < 2 {
		return 0
	}
	first := w.samples[0]
	last := w.samples[len(w.samples)-1]
	elapsed := last.at.Sub(first.at).Seconds()
	delta := last.bytes - first.bytes
	if elapsed <= 0 || delta <= 0 {
		return 0
	}
	return int64(float64(delta) / elapsed)
}

type rateState struct {
	down *rateWindow
	up   *rateWindow
}

func newRateState() *rateState {
	return &rateState{down: newRateWindow(defaultRateSpan), up: newRateWindow(defaultRateSpan)}
}

func etaSeconds(remaining, bytesPerSecond int64) int64 {
	if remaining <= 0 {
		return 0
	}
	if bytesPerSecond <= 0 {
		return -1
	}
	return remaining / bytesPerSecond
}

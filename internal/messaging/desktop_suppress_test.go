package messaging

import (
	"sync"
	"testing"
)

func TestDesktopSuppressed(t *testing.T) {
	cases := []struct {
		name string
		fn   func() bool
		want bool
	}{
		{"no check installed", nil, false},
		{"check says visible", func() bool { return true }, true},
		{"check says hidden", func() bool { return false }, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d := &Desktop{}
			d.SetSuppress(c.fn)
			if got := d.suppressed(); got != c.want {
				t.Fatalf("suppressed = %v, want %v", got, c.want)
			}
		})
	}
}

func TestDesktopSuppressRaceWithNotify(t *testing.T) {
	d := &Desktop{}
	var wg sync.WaitGroup
	for range 4 {
		wg.Add(2)
		go func() {
			defer wg.Done()
			for range 200 {
				d.SetSuppress(func() bool { return true })
			}
		}()
		go func() {
			defer wg.Done()
			for range 200 {
				d.suppressed()
			}
		}()
	}
	wg.Wait()
}

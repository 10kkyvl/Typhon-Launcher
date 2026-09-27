package idle

import (
	"testing"
	"time"
)

func TestSinceReportsAPlausibleTime(t *testing.T) {
	d, ok := Since()
	if !ok {
		if d != 0 {
			t.Fatalf("unknown idle time came back as %v, want 0", d)
		}
		return
	}
	if d < 0 {
		t.Fatalf("idle time is negative: %v", d)
	}
	if d > 30*24*time.Hour {
		t.Fatalf("idle time is implausible: %v", d)
	}
}

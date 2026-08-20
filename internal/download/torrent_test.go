package download

import (
	"errors"
	"fmt"
	"net"
	"runtime"
	"testing"

	"golang.org/x/time/rate"
)

func TestIsSafeTorrentPath(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{"game/data/pak0.pak", true},
		{"setup.exe", true},
		{"", false},
		{"   ", false},
		{"../x", false},
		{`C:\x`, false},
		{"a/../../b", false},
		{"a//b", false},
		{"./b", false},
		{`dir\file`, false},
	}
	for _, c := range cases {
		if got := isSafeTorrentPath(c.path); got != c.want {
			t.Fatalf("isSafeTorrentPath(%q) = %v, want %v", c.path, got, c.want)
		}
	}
	if runtime.GOOS == "windows" && isSafeTorrentPath("con") {
		t.Fatal(`isSafeTorrentPath("con") = true, want false`)
	}
}

func TestLimiterBurst(t *testing.T) {
	if got := limiterBurst(1024); got != minLimiterBurst {
		t.Fatalf("burst = %d, want %d", got, minLimiterBurst)
	}
	if got := limiterBurst(4 << 20); got != 4<<20 {
		t.Fatalf("burst = %d, want %d", got, 4<<20)
	}
}

func TestIsListenError(t *testing.T) {
	bindErr := &net.OpError{
		Op:  "listen",
		Net: "udp4",
		Err: errors.New("Only one usage of each socket address is normally permitted."),
	}
	if !isListenError(bindErr) {
		t.Fatal("net.OpError not recognised as a listen failure")
	}
	if !isListenError(fmt.Errorf("wrapped: %w", bindErr)) {
		t.Fatal("wrapped net.OpError not recognised")
	}
	if !isListenError(errors.New("listen udp4 :42815: bind: address already in use")) {
		t.Fatal("plain bind error not recognised")
	}
	if isListenError(errors.New("не удалось прочитать torrent-файл")) {
		t.Fatal("unrelated error treated as a listen failure")
	}
}

func TestApplyLimit(t *testing.T) {
	l := newLimiter(0)
	if l.Limit() != rate.Inf {
		t.Fatalf("limit = %v, want Inf", l.Limit())
	}
	applyLimit(l, 2<<20)
	if l.Limit() != rate.Limit(2<<20) || l.Burst() != 2<<20 {
		t.Fatalf("limit = %v burst = %d", l.Limit(), l.Burst())
	}
	applyLimit(l, 0)
	if l.Limit() != rate.Inf {
		t.Fatalf("limit = %v, want Inf", l.Limit())
	}
}

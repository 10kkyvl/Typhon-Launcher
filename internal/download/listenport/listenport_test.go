package listenport

import (
	"errors"
	"fmt"
	"net"
	"testing"
)

func TestIsListenError(t *testing.T) {
	bindErr := &net.OpError{
		Op:  "listen",
		Net: "udp4",
		Err: errors.New("Only one usage of each socket address is normally permitted."),
	}
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"net.OpError", bindErr, true},
		{"wrapped net.OpError", fmt.Errorf("wrapped: %w", bindErr), true},
		{"plain bind error", errors.New("listen udp4 :42815: bind: address already in use"), true},
		{"unrelated error", errors.New("не удалось прочитать torrent-файл"), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := IsListenError(c.err); got != c.want {
				t.Fatalf("IsListenError(%v) = %v, want %v", c.err, got, c.want)
			}
		})
	}
}

func TestRandomStaysInTheDynamicRange(t *testing.T) {
	seen := map[int]bool{}
	for range 2000 {
		p, err := Random()
		if err != nil {
			t.Fatal(err)
		}
		if p < First || p > 65535 {
			t.Fatalf("port %d is outside the dynamic range", p)
		}
		seen[p] = true
	}
	if len(seen) < 1000 {
		t.Fatalf("2000 draws gave %d different ports: the pick is not random", len(seen))
	}
}

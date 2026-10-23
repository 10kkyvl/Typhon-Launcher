package main

import (
	"errors"
	"testing"
)

type fakeTrayPlayer struct {
	played []string
	err    error
}

func (p *fakeTrayPlayer) PlayGame(id string) error {
	p.played = append(p.played, id)
	return p.err
}

type fakeTrayStopper struct {
	stopped []string
	err     error
}

func (s *fakeTrayStopper) StopGame(id string) error {
	s.stopped = append(s.stopped, id)
	return s.err
}

func TestPlayFromTray(t *testing.T) {
	cases := []struct {
		name       string
		err        error
		wantReveal int
	}{
		{"launched game leaves the window alone", nil, 0},
		{"a refused launch opens the window", errors.New("already running"), 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			player := &fakeTrayPlayer{err: tc.err}
			revealed := 0
			playFromTray(player, "g1", func() { revealed++ })
			if len(player.played) != 1 || player.played[0] != "g1" {
				t.Fatalf("PlayGame calls = %v, want [g1]", player.played)
			}
			if revealed != tc.wantReveal {
				t.Fatalf("window opened %d times, want %d", revealed, tc.wantReveal)
			}
		})
	}
}

func TestStopFromTray(t *testing.T) {
	cases := []struct {
		name       string
		err        error
		wantReveal int
	}{
		{"stopped game leaves the window alone", nil, 0},
		{"a failed stop opens the window", errors.New("process gone"), 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stopper := &fakeTrayStopper{err: tc.err}
			revealed := 0
			stopFromTray(stopper, "g1", func() { revealed++ })
			if len(stopper.stopped) != 1 || stopper.stopped[0] != "g1" {
				t.Fatalf("StopGame calls = %v, want [g1]", stopper.stopped)
			}
			if revealed != tc.wantReveal {
				t.Fatalf("window opened %d times, want %d", revealed, tc.wantReveal)
			}
		})
	}
}

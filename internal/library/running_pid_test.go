package library

import "testing"

func TestRunningGameForPID(t *testing.T) {
	s := &Service{running: map[string]*session{
		"a": {pid: 42},
		"b": {pid: 7},
	}}
	cases := []struct {
		name   string
		pid    uint32
		wantID string
		wantOK bool
	}{
		{"first game", 42, "a", true},
		{"second game", 7, "b", true},
		{"unknown pid", 99, "", false},
		{"zero pid", 0, "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			id, ok := s.RunningGameForPID(c.pid)
			if id != c.wantID || ok != c.wantOK {
				t.Fatalf("RunningGameForPID(%d) = %q, %v; want %q, %v", c.pid, id, ok, c.wantID, c.wantOK)
			}
		})
	}
}

func TestRunningGameForPIDWithNothingRunning(t *testing.T) {
	s := &Service{running: map[string]*session{}}
	if id, ok := s.RunningGameForPID(42); ok || id != "" {
		t.Fatalf("got %q, %v with no running games", id, ok)
	}
}

package library

import (
	"reflect"
	"testing"
)

func boolPtr(v bool) *bool { return &v }

func TestSetGameModeOverrideStoresAndClears(t *testing.T) {
	cases := []struct {
		name string
		in   GameModeOverride
		want *GameModeOverride
	}{
		{name: "raise only", in: GameModeOverride{RaisePriority: boolPtr(true)}, want: &GameModeOverride{RaisePriority: boolPtr(true)}},
		{name: "explicit false is kept", in: GameModeOverride{HighPerformance: boolPtr(false)}, want: &GameModeOverride{HighPerformance: boolPtr(false)}},
		{name: "all inherit clears", in: GameModeOverride{}, want: nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, game := propertiesService(t)
			got, err := s.SetGameModeOverride(game.ID, tc.in)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got.GameMode, tc.want) {
				t.Fatalf("returned = %+v, want %+v", got.GameMode, tc.want)
			}
			reloaded, err := NewServiceAt(s.path)
			if err != nil {
				t.Fatal(err)
			}
			stored, err := reloaded.Find(game.ID)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(stored.GameMode, tc.want) {
				t.Fatalf("reloaded = %+v, want %+v", stored.GameMode, tc.want)
			}
		})
	}
}

func TestSetGameModeOverrideRollsBackOnPersistFailure(t *testing.T) {
	s, game := propertiesService(t)
	if _, err := s.SetGameModeOverride(game.ID, GameModeOverride{HideLauncher: boolPtr(true)}); err != nil {
		t.Fatal(err)
	}
	blockPersist(t, s)
	if _, err := s.SetGameModeOverride(game.ID, GameModeOverride{HideLauncher: boolPtr(false), RaisePriority: boolPtr(true)}); err == nil {
		t.Fatal("expected persist failure")
	}
	after, err := s.Find(game.ID)
	if err != nil {
		t.Fatal(err)
	}
	want := &GameModeOverride{HideLauncher: boolPtr(true)}
	if !reflect.DeepEqual(after.GameMode, want) {
		t.Fatalf("override after failed persist = %+v, want %+v", after.GameMode, want)
	}
}

func TestSetGameModeOverrideUnknownGame(t *testing.T) {
	s, _ := propertiesService(t)
	if _, err := s.SetGameModeOverride("missing", GameModeOverride{}); err == nil {
		t.Fatal("expected not found")
	}
}

func TestRunningPID(t *testing.T) {
	s, game := propertiesService(t)
	if _, ok := s.RunningPID(game.ID); ok {
		t.Fatal("pid reported for a game that is not running")
	}
	s.mu.Lock()
	s.running[game.ID] = &session{pid: 4242}
	s.mu.Unlock()
	pid, ok := s.RunningPID(game.ID)
	if !ok || pid != 4242 {
		t.Fatalf("RunningPID = %d, %v", pid, ok)
	}
	s.mu.Lock()
	s.running[game.ID] = &session{}
	s.mu.Unlock()
	if _, ok := s.RunningPID(game.ID); ok {
		t.Fatal("zero pid reported as known")
	}
}

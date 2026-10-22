package main

import (
	"errors"
	"testing"
)

func TestPlayTarget(t *testing.T) {
	cases := []struct {
		name      string
		args      []string
		want      playRequest
		requested bool
		err       error
	}{
		{"none", []string{"typhon.exe"}, playRequest{}, false, nil},
		{"play", []string{"typhon.exe", "--play", "g1"}, playRequest{id: "g1"}, true, nil},
		{"play profile", []string{"typhon.exe", "--play", "g1", "--profile", "ab12cd34"}, playRequest{id: "g1", profile: "ab12cd34"}, true, nil},
		{"profile first", []string{"typhon.exe", "--profile", "ab12cd34", "--play", "g1"}, playRequest{id: "g1", profile: "ab12cd34"}, true, nil},
		{"no id", []string{"typhon.exe", "--play"}, playRequest{}, true, errNoPlayTarget},
		{"blank id", []string{"typhon.exe", "--play", " "}, playRequest{}, true, errNoPlayTarget},
		{"profile without value", []string{"typhon.exe", "--play", "g1", "--profile"}, playRequest{}, true, errBadProfileTarget},
		{"profile blank", []string{"typhon.exe", "--play", "g1", "--profile", ""}, playRequest{}, true, errBadProfileTarget},
		{"profile flag-like", []string{"typhon.exe", "--play", "g1", "--profile", "--play"}, playRequest{}, true, errBadProfileTarget},
		{"profile bad chars", []string{"typhon.exe", "--play", "g1", "--profile", "a b;c"}, playRequest{}, true, errBadProfileTarget},
		{"profile without play", []string{"typhon.exe", "--profile", "ab12cd34"}, playRequest{}, false, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, requested, err := playTarget(tc.args)
			if !errors.Is(err, tc.err) || requested != tc.requested || got != tc.want {
				t.Fatalf("playTarget = %+v, %v, %v; want %+v, %v, %v", got, requested, err, tc.want, tc.requested, tc.err)
			}
		})
	}
}

func TestPlayFromShortcutPassesProfile(t *testing.T) {
	player := &fakeGamePlayer{}
	playFromShortcut(player, playRequest{id: "g1", profile: "ab12cd34"}, func() {})
	if len(player.profs) != 1 || player.profs[0] != "ab12cd34" {
		t.Fatalf("profiles = %v", player.profs)
	}
}

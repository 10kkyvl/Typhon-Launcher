package main

import (
	"errors"
	"testing"
)

type fakeGamePlayer struct {
	err   error
	calls int
	ids   []string
}

func (f *fakeGamePlayer) PlayGame(id string) error {
	f.calls++
	f.ids = append(f.ids, id)
	return f.err
}

func TestPlayFromShortcutRevealsOnFailure(t *testing.T) {
	player := &fakeGamePlayer{err: errors.New("library.cannot_confirm_process")}
	revealed := 0

	playFromShortcut(player, "game-1", func() { revealed++ })

	if player.calls != 1 || len(player.ids) != 1 || player.ids[0] != "game-1" {
		t.Fatalf("PlayGame calls = %d %v, want exactly one call for game-1", player.calls, player.ids)
	}
	if revealed != 1 {
		t.Fatalf("reveal called %d times, want exactly 1 for a failed launch", revealed)
	}
}

func TestPlayFromShortcutDoesNotRevealOnSuccess(t *testing.T) {
	player := &fakeGamePlayer{}
	revealed := 0

	playFromShortcut(player, "game-1", func() { revealed++ })

	if player.calls != 1 {
		t.Fatalf("PlayGame calls = %d, want 1", player.calls)
	}
	if revealed != 0 {
		t.Fatalf("reveal called %d times, want 0 for a successful launch", revealed)
	}
}

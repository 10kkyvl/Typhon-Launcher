package presence

import (
	"testing"

	"typhon/internal/library"
)

func TestHiddenGameIsNotShown(t *testing.T) {
	w, target := newWatcher(t, noCover)
	w.SessionStarted(library.Game{ID: "1", Title: "Secret Game", Hidden: true})
	if len(target.shown) != 0 {
		t.Fatalf("shown = %+v, want nothing for a hidden game", target.shown)
	}
}

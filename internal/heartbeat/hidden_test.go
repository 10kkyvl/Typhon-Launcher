package heartbeat

import (
	"net/http"
	"testing"

	"typhon/internal/library"
)

func TestSessionStartedIgnoresHiddenGames(t *testing.T) {
	cases := []struct {
		name string
		game library.Game
		want bool
	}{
		{"visible", library.Game{ID: "g1", CanonicalGameID: "42"}, true},
		{"hidden", library.Game{ID: "g1", CanonicalGameID: "42", Hidden: true}, false},
		{"hide presence", library.Game{ID: "g1", CanonicalGameID: "42", HidePresence: true}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv, _ := newCapturingServer(t, http.StatusNoContent)
			svc := newTestService(t, srv, func(id string) string { return id })
			svc.SessionStarted(tc.game)
			svc.mu.Lock()
			_, tracked := svc.running[tc.game.ID]
			svc.mu.Unlock()
			if tracked != tc.want {
				t.Fatalf("running[%q] = %v, want %v", tc.game.ID, tracked, tc.want)
			}
		})
	}
}

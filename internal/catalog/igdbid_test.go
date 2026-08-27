package catalog

import "testing"

func TestServiceIGDBIDOf(t *testing.T) {
	s := newTestService(t)
	games := seed(t, s,
		Game{Title: "Cyberpunk 2077", ExternalIDs: ExternalIDs{IGDB: "1877"}},
		Game{Title: "Portal 2"},
	)
	withIGDB := games[0]
	withoutIGDB := games[1]

	cases := []struct {
		name string
		id   string
		want string
	}{
		{"has igdb id", withIGDB.ID, "1877"},
		{"empty igdb id on game", withoutIGDB.ID, ""},
		{"unknown game id", "does-not-exist", ""},
		{"empty input id", "", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := s.IGDBIDOf(tc.id)
			if got != tc.want {
				t.Fatalf("IGDBIDOf(%q) = %q, want %q", tc.id, got, tc.want)
			}
		})
	}
}

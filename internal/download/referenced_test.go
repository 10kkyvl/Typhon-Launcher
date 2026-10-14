package download

import (
	"slices"
	"testing"
)

func TestReferencedGameIDs(t *testing.T) {
	cases := []struct {
		name  string
		items map[string]Status
		games map[string]string
		want  []string
	}{
		{name: "empty", want: []string{}},
		{
			name:  "empty ids are skipped",
			items: map[string]Status{"a": StatusQueued, "b": StatusCompleted},
			games: map[string]string{"a": "", "b": "g1"},
			want:  []string{"g1"},
		},
		{
			name:  "duplicates collapse and every status counts, sorted",
			items: map[string]Status{"a": StatusQueued, "b": StatusCompleted, "c": StatusFailed, "d": StatusDownloading},
			games: map[string]string{"a": "zz", "b": "aa", "c": "zz", "d": "mm"},
			want:  []string{"aa", "mm", "zz"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := newTestManager(t, 2)
			for id, status := range tc.items {
				d := m.addTestItem(id, status)
				m.mu.Lock()
				d.Origin.GameID = tc.games[id]
				m.mu.Unlock()
			}
			got := m.ReferencedGameIDs()
			if got == nil || !slices.Equal(got, tc.want) {
				t.Fatalf("ReferencedGameIDs() = %#v, want %#v", got, tc.want)
			}
		})
	}
}

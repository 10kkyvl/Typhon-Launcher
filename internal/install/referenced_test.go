package install

import (
	"slices"
	"testing"

	"typhon/internal/download"
)

func TestReferencedGameIDs(t *testing.T) {
	cases := []struct {
		name string
		ids  []string
		want []string
	}{
		{name: "empty", want: []string{}},
		{name: "empty ids are skipped", ids: []string{"", "g1", ""}, want: []string{"g1"}},
		{name: "duplicates collapse, sorted", ids: []string{"zz", "aa", "zz", "mm", "aa"}, want: []string{"aa", "mm", "zz"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, _, _ := newTestService(t)
			s.mu.Lock()
			for _, id := range tc.ids {
				s.items = append(s.items, &Installation{Origin: download.Origin{GameID: id}})
			}
			s.mu.Unlock()
			got := s.ReferencedGameIDs()
			if got == nil || !slices.Equal(got, tc.want) {
				t.Fatalf("ReferencedGameIDs() = %#v, want %#v", got, tc.want)
			}
		})
	}
}

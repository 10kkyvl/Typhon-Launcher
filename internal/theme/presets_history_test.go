package theme

import (
	"maps"
	"slices"
	"testing"
)

// A generation is a snapshot of the whole built-in set at one release, so
// every preset in it carries the same token names, all of them themeable.
func TestReleasedPresetGenerationsAreWholeSnapshots(t *testing.T) {
	if len(releasedPresetGenerations) == 0 {
		t.Fatal("no released preset generations")
	}
	for _, gen := range releasedPresetGenerations {
		ids := slices.Sorted(maps.Keys(gen.presets))
		var want []string
		for _, p := range presets {
			want = append(want, p.ID)
		}
		slices.Sort(want)
		if !slices.Equal(ids, want) {
			t.Errorf("%s: presets %v, want %v", gen.releases, ids, want)
		}
		var names []string
		for id, tokens := range gen.presets {
			got := slices.Sorted(maps.Keys(tokens))
			if names == nil {
				names = got
			} else if !slices.Equal(got, names) {
				t.Errorf("%s: %s has tokens %v, the others %v", gen.releases, id, got, names)
			}
			for _, name := range got {
				if _, ok := allowedTokenSet[name]; !ok || isSettingsOwned(name) {
					t.Errorf("%s: %s carries %s, which no theme may set", gen.releases, id, name)
				}
			}
		}
	}
}

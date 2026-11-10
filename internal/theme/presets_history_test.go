package theme

import (
	"maps"
	"slices"
	"testing"
)

// A generation is a snapshot of the whole built-in set at one release, so
// every preset in it carries the same token names, all of them themeable. A
// preset added later is absent from the older generations, while a released
// one stays built in: its forks are migrated against it.
func TestReleasedPresetGenerationsAreWholeSnapshots(t *testing.T) {
	if len(releasedPresetGenerations) == 0 {
		t.Fatal("no released preset generations")
	}
	for _, gen := range releasedPresetGenerations {
		if _, ok := gen.presets[defaultTheme().ID]; !ok {
			t.Errorf("%s: no %s preset, which forks are matched against", gen.releases, defaultTheme().ID)
		}
		for id := range gen.presets {
			if _, ok := presetByID(id); !ok {
				t.Errorf("%s: released preset %s is no longer built in", gen.releases, id)
			}
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

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
	for _, lost := range droppedPresets(releasedPresetGenerations) {
		t.Error(lost)
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

// droppedPresets lists every preset a generation lost against the one before
// it: a built-in preset, once released, stays in every later generation.
func droppedPresets(gens []presetGeneration) []string {
	var lost []string
	for i := 1; i < len(gens); i++ {
		for _, id := range slices.Sorted(maps.Keys(gens[i-1].presets)) {
			if _, ok := gens[i].presets[id]; !ok {
				lost = append(lost, gens[i].releases+": lost preset "+id+" of "+gens[i-1].releases)
			}
		}
	}
	return lost
}

func TestDroppedPresetsSeesAShrinkingGeneration(t *testing.T) {
	one := map[string]string{"--bg": "#000000"}
	gens := []presetGeneration{
		{releases: "a", presets: map[string]map[string]string{"dark": one, "light": one}},
		{releases: "b", presets: map[string]map[string]string{"dark": one, "light": one, "blush": one}},
		{releases: "c", presets: map[string]map[string]string{"dark": one, "blush": one}},
	}
	got := droppedPresets(gens)
	if want := []string{"c: lost preset light of b"}; !slices.Equal(got, want) {
		t.Fatalf("droppedPresets() = %v, want %v", got, want)
	}
}

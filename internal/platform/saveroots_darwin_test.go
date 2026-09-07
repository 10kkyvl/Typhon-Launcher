//go:build darwin

package platform

import (
	"path/filepath"
	"testing"

	"typhon/internal/wine"
)

func TestSaveRootsFromCoversBottleAndHome(t *testing.T) {
	bottle := wine.Bottle{Name: "B", Path: "/bottles/B", Drive: "t", Games: "/Users/x/Games"}

	got := saveRootsFrom("/Users/x", []wine.Bottle{bottle})

	user := filepath.Join("/bottles/B", "drive_c", "users", "crossover")
	want := map[string]int{
		filepath.Join(user, "Saved Games"):                 1,
		filepath.Join(user, "AppData", "Roaming"):          2,
		filepath.Join(user, "AppData", "Local"):            2,
		filepath.Join(user, "AppData", "LocalLow"):         2,
		filepath.Join("/Users/x", "Documents", "My Games"): 2,
		filepath.Join("/Users/x", "Documents"):             1,
	}
	if len(got) != len(want) {
		t.Fatalf("got %d roots, want %d: %+v", len(got), len(want), got)
	}
	for _, root := range got {
		depth, ok := want[root.Path]
		if !ok {
			t.Fatalf("unexpected root %q", root.Path)
		}
		if root.Depth != depth {
			t.Fatalf("root %q depth = %d, want %d", root.Path, root.Depth, depth)
		}
	}
}

// Documents внутри бутыля — симлинк на настоящий ~/Documents, поэтому общий
// корень добавляется один раз, сколько бы бутылей ни было.
func TestSaveRootsFromDoesNotRepeatHome(t *testing.T) {
	bottles := []wine.Bottle{
		{Name: "A", Path: "/bottles/A", Drive: "t", Games: "/Users/x/Games"},
		{Name: "B", Path: "/bottles/B", Drive: "u", Games: "/Users/x/Games"},
	}

	got := saveRootsFrom("/Users/x", bottles)

	documents := 0
	for _, root := range got {
		if root.Path == filepath.Join("/Users/x", "Documents") {
			documents++
		}
	}
	if documents != 1 {
		t.Fatalf("Documents appears %d times, want 1", documents)
	}
}

func TestSaveRootsFromWithoutBottles(t *testing.T) {
	got := saveRootsFrom("/Users/x", nil)
	if len(got) != 2 {
		t.Fatalf("got %+v, want only the two shared Documents roots", got)
	}
}

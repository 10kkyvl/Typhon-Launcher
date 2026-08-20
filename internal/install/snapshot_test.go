package install

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSnapshotDiffFindsNewDirectory(t *testing.T) {
	root := t.TempDir()
	mkFile(t, filepath.Join(root, "Existing", "file.txt"), 4)

	before := takeSnapshot([]string{root, filepath.Join(root, "missing")})
	if len(before.roots) != 1 {
		t.Fatalf("roots = %v", before.roots)
	}

	fresh := filepath.Join(root, "Fresh")
	mkFile(t, filepath.Join(fresh, "game.exe"), 8)

	got := diffSnapshot(before, takeSnapshot([]string{root}))
	if len(got) != 1 || got[0] != fresh {
		t.Fatalf("diff = %v, want [%s]", got, fresh)
	}
}

func TestSnapshotDiffSkipsRootsAndNestedDirs(t *testing.T) {
	root := t.TempDir()
	before := takeSnapshot([]string{root})

	nested := filepath.Join(root, "Game", "bin")
	mkFile(t, filepath.Join(nested, "game.exe"), 8)

	got := diffSnapshot(before, takeSnapshot([]string{root}))
	if len(got) != 1 || got[0] != filepath.Join(root, "Game") {
		t.Fatalf("diff = %v", got)
	}
}

func TestSnapshotDiffDetectsChangedChildren(t *testing.T) {
	root := t.TempDir()
	existing := filepath.Join(root, "Existing")
	mkFile(t, filepath.Join(existing, "one.txt"), 4)

	before := takeSnapshot([]string{root})
	mkFile(t, filepath.Join(existing, "two.txt"), 4)

	got := diffSnapshot(before, takeSnapshot([]string{root}))
	if len(got) != 1 || got[0] != existing {
		t.Fatalf("diff = %v, want [%s]", got, existing)
	}
}

func TestSnapshotDiffIsEmptyWithoutChanges(t *testing.T) {
	root := t.TempDir()
	mkFile(t, filepath.Join(root, "Existing", "file.txt"), 4)

	before := takeSnapshot([]string{root})
	if got := diffSnapshot(before, takeSnapshot([]string{root})); len(got) != 0 {
		t.Fatalf("diff = %v", got)
	}
}

func TestSnapshotStopsAtDepthLimit(t *testing.T) {
	root := t.TempDir()
	deep := filepath.Join(root, "a", "b", "c", "d")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	snap := takeSnapshot([]string{root})
	if _, ok := snap.dirs[filepath.Join(root, "a", "b")]; !ok {
		t.Fatal("depth 2 not recorded")
	}
	if _, ok := snap.dirs[filepath.Join(root, "a", "b", "c")]; ok {
		t.Fatal("walked past the depth limit")
	}
}

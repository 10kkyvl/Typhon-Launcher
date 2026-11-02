package emulators

import (
	"os/exec"
	"path/filepath"
	"testing"
)

func junction(t *testing.T, link, target string) {
	t.Helper()
	//nolint:gosec // G204: аргументы — пути из t.TempDir(); внешнего ввода, который требует валидации по инварианту 32, здесь нет
	if out, err := exec.Command("cmd", "/c", "mklink", "/J", link, target).CombinedOutput(); err != nil {
		t.Fatalf("mklink: %v: %s", err, out)
	}
}

// The line a real Vita3K 0.2.1 wrote: forward and back slashes in one plain value.
func TestPrefPathOfARealConfig(t *testing.T) {
	data := []byte(`pref-path: E:/tmp/home\AppData\Roaming\Vita3K\Vita3K\` + "\n")
	got, ok := prefPath(data)
	if !ok || filepath.Clean(got) != `E:\tmp\home\AppData\Roaming\Vita3K\Vita3K` {
		t.Fatalf("prefPath = %q, %v", got, ok)
	}
}

func TestPlaceSeesThroughJunctionsAndDots(t *testing.T) {
	vita := mustPlatform(t, Vita)
	root := t.TempDir()
	known := Storage{Root: root, Known: true}
	dest := filepath.Join(root, "ux0", "app", "PCSE00001")
	plant(t, filepath.Join(dest, "eboot.bin"), []byte("x"))

	other := t.TempDir()
	link := filepath.Join(other, "games")
	junction(t, link, dest)
	if got := vita.Place(filepath.Join(link, "eboot.bin"), "PCSE00001", known); got != PlaceInstalled {
		t.Fatalf("through a junction: %v", got)
	}

	moved := filepath.Join(t.TempDir(), "elsewhere")
	storageLink := filepath.Join(root, "ux0", "patch")
	plant(t, filepath.Join(moved, "PCSE00001", "eboot.bin"), []byte("x"))
	junction(t, storageLink, moved)
	if got := vita.Place(filepath.Join(moved, "PCSE00001", "eboot.bin"), "PCSE00001", known); got != PlaceData {
		t.Fatalf("a storage folder that is itself a junction: %v", got)
	}

	dotted := root + `\ux0.\app\PCSE00001\eboot.bin`
	if got := vita.Place(dotted, "PCSE00001", known); got != PlaceInstalled {
		t.Fatalf("a folder with a trailing dot: %v", got)
	}
	unrelated := filepath.Join(t.TempDir(), "junk")
	plant(t, filepath.Join(unrelated, "eboot.bin"), []byte("x"))
	if got := vita.Place(filepath.Join(unrelated, "eboot.bin"), "PCSE00001", known); got != PlaceOutside {
		t.Fatalf("an unrelated folder: %v", got)
	}
}

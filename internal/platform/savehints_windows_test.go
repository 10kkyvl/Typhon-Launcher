package platform

import (
	"context"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestResolveSaveHintSkipsJunctions(t *testing.T) {
	root := t.TempDir()
	mkdirs(t, root, "outside/secret", "real/Saves")
	junction := func(link, target string) {
		t.Helper()
		//nolint:gosec // G204: test-only, fixed program and arguments, paths come from t.TempDir (invariant 33 concerns launcher binaries)
		if out, err := exec.Command("cmd", "/c", "mklink", "/J", link, target).CombinedOutput(); err != nil {
			t.Fatalf("mklink /J: %v: %s", err, out)
		}
	}
	junction(filepath.Join(root, "real", "Link"), filepath.Join(root, "outside"))
	junction(filepath.Join(root, "gamelink"), filepath.Join(root, "real"))
	env := SaveEnv{TokenAppData: filepath.Join(root, "real"), TokenGame: filepath.Join(root, "gamelink")}

	for _, hint := range []string{"{appdata}/Link/secret", "{appdata}/Li*/secret"} {
		paths, unreadable, err := ResolveSaveHint(context.Background(), hint, env)
		if err != nil || len(paths) != 0 || unreadable != 1 {
			t.Fatalf("%s: %v %d %v, want the junction skipped and counted", hint, paths, unreadable, err)
		}
	}
	paths, unreadable, err := ResolveSaveHint(context.Background(), "{game}/Saves", env)
	if err != nil || len(paths) != 0 || unreadable != 1 {
		t.Fatalf("game root junction: %v %d %v", paths, unreadable, err)
	}
}

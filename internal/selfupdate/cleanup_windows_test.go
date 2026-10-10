//go:build windows

package selfupdate

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestCleanupCacheDoesNotFollowAJunctionOutOfTheCache(t *testing.T) {
	dir := t.TempDir()
	cacheDir, err := CacheDir(dir)
	if err != nil {
		t.Fatalf("CacheDir: %v", err)
	}
	outside := filepath.Join(t.TempDir(), "outside")
	precious := filepath.Join(outside, "precious.txt")
	writeTestFile(t, precious, []byte("not the updater's to delete"))
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	link := filepath.Join(cacheDir, "9.9.9")
	cmdExe := filepath.Join(os.Getenv("SystemRoot"), "System32", "cmd.exe")
	//nolint:gosec // G204: invariant 33, cmd.exe is resolved from %SystemRoot% and the remaining arguments are temp directories created by this test
	if out, err := exec.Command(cmdExe, "/c", "mklink", "/J", link, outside).CombinedOutput(); err != nil {
		t.Fatalf("mklink /J: %v: %s", err, out)
	}

	s := &Service{dir: dir, notes: mustNotesStore(t, dir), currentVersion: "1.0.0"}
	if err := s.cleanupCache(context.Background(), ""); err != nil {
		t.Fatalf("cleanupCache: %v", err)
	}

	if data, err := os.ReadFile(precious); err != nil || string(data) != "not the updater's to delete" {
		t.Fatalf("file behind the junction = %q, %v; the sweep followed the link out of the cache", data, err)
	}
	if _, err := os.Lstat(link); err == nil {
		t.Fatal("the junction itself survived the sweep")
	}
}

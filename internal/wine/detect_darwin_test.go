package wine

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// fakeCrossOver собирает дерево, повторяющее раскладку CrossOver.app: тесты
// не могут зависеть от того, установлен ли CrossOver на машине сборки.
func fakeCrossOver(t *testing.T, version string) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "CrossOver.app")
	bin := filepath.Join(root, "Contents", "SharedSupport", "CrossOver", "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	for _, name := range []string{"cxbottle", "cxstart", "wineserver"} {
		path := filepath.Join(bin, name)
		if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
			t.Fatalf("WriteFile %s: %v", name, err)
		}
	}
	plist := filepath.Join(root, "Contents", "Info.plist")
	body := `<?xml version="1.0" encoding="UTF-8"?>
<plist version="1.0"><dict>
<key>CFBundleShortVersionString</key><string>` + version + `</string>
</dict></plist>`
	if err := os.WriteFile(plist, []byte(body), 0o644); err != nil {
		t.Fatalf("WriteFile plist: %v", err)
	}
	return root
}

func TestDetectAtFindsRuntime(t *testing.T) {
	root := fakeCrossOver(t, "26.3")

	rt, err := detectAt(root)
	if err != nil {
		t.Fatalf("detectAt: %v", err)
	}
	if rt.Version != "26.3" {
		t.Fatalf("Version = %q, want %q", rt.Version, "26.3")
	}
	if filepath.Base(rt.CxBottle) != "cxbottle" {
		t.Fatalf("CxBottle = %q", rt.CxBottle)
	}
	if filepath.Base(rt.CxStart) != "cxstart" {
		t.Fatalf("CxStart = %q", rt.CxStart)
	}
	if filepath.Base(rt.WineServer) != "wineserver" {
		t.Fatalf("WineServer = %q", rt.WineServer)
	}
}

func TestDetectAtMissingIsNotInstalled(t *testing.T) {
	_, err := detectAt(filepath.Join(t.TempDir(), "nope.app"))
	if !errors.Is(err, ErrNotInstalled) {
		t.Fatalf("err = %v, want ErrNotInstalled", err)
	}
}

func TestDetectAtIncompleteIsNotInstalled(t *testing.T) {
	root := fakeCrossOver(t, "26.3")
	bin := filepath.Join(root, "Contents", "SharedSupport", "CrossOver", "bin")
	if err := os.Remove(filepath.Join(bin, "cxstart")); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	_, err := detectAt(root)
	if !errors.Is(err, ErrNotInstalled) {
		t.Fatalf("err = %v, want ErrNotInstalled", err)
	}
}

func TestDetectAtVersionOptional(t *testing.T) {
	root := fakeCrossOver(t, "26.3")
	if err := os.Remove(filepath.Join(root, "Contents", "Info.plist")); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	rt, err := detectAt(root)
	if err != nil {
		t.Fatalf("detectAt without plist: %v", err)
	}
	if rt.Version != "" {
		t.Fatalf("Version = %q, want empty", rt.Version)
	}
}

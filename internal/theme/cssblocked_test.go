package theme

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"typhon/internal/storage"
)

const storedBypassCSS = `.a { background-image: image-set("https://evil.test/x.png" 1x); }`

func writeStoredThemes(t *testing.T, themes []Theme, active string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "themes.json")
	if err := storage.Save(path, stateVersion, state{Themes: themes, Confirmed: active}); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestStoredThemeWithRefusedCSSIsServedWithoutIt(t *testing.T) {
	blocked := validTheme("old-share")
	blocked.CSS = storedBypassCSS
	clean := validTheme("old-clean")
	path := writeStoredThemes(t, []Theme{blocked, clean}, "old-share")

	s := mustServiceAt(t, path)

	active, err := s.Active()
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.Get("old-share")
	if err != nil {
		t.Fatal(err)
	}
	var listed Theme
	for _, th := range s.List() {
		if th.ID == "old-share" {
			listed = th
		}
	}
	for name, th := range map[string]Theme{"Active": active, "Get": got, "List": listed} {
		if th.CSS != "" || !th.CSSBlocked {
			t.Errorf("%s: css = %q, blocked = %v; want empty css and blocked", name, th.CSS, th.CSSBlocked)
		}
	}
	other, err := s.Get("old-clean")
	if err != nil {
		t.Fatal(err)
	}
	if other.CSS != clean.CSS || other.CSSBlocked {
		t.Fatalf("clean theme: css = %q, blocked = %v", other.CSS, other.CSSBlocked)
	}

	if err := s.Apply("old-clean"); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		t.Fatal(err)
	}
	var onDisk struct {
		Data state `json:"data"`
	}
	if err := json.Unmarshal(raw, &onDisk); err != nil {
		t.Fatal(err)
	}
	if len(onDisk.Data.Themes) != 2 || onDisk.Data.Themes[0].CSS != storedBypassCSS {
		t.Fatalf("persist after load lost the stored css: %s", raw)
	}

	out := filepath.Join(t.TempDir(), "out.typhontheme")
	if err := s.Export("old-share", out); err != nil {
		t.Fatal(err)
	}
	exported, err := os.ReadFile(filepath.Clean(out))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(exported), "image-set") || strings.Contains(string(exported), "cssBlocked") {
		t.Fatalf("export carries the refused css: %s", exported)
	}
}

func TestStoredBlockedFlagIsRecomputed(t *testing.T) {
	stale := validTheme("stale-flag")
	stale.CSSBlocked = true
	s := mustServiceAt(t, writeStoredThemes(t, []Theme{stale}, ""))
	got, err := s.Get("stale-flag")
	if err != nil {
		t.Fatal(err)
	}
	if got.CSSBlocked || got.CSS != stale.CSS {
		t.Fatalf("css = %q, blocked = %v; a valid css must be served", got.CSS, got.CSSBlocked)
	}
}

func TestValidateClearsCallerBlockedFlag(t *testing.T) {
	th := validTheme("caller-flag")
	th.CSSBlocked = true
	got, err := Validate(th)
	if err != nil {
		t.Fatal(err)
	}
	if got.CSSBlocked {
		t.Fatal("Validate kept a caller-supplied cssBlocked")
	}
}

package theme

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"typhon/internal/storage"
)

const shadowedName = "Мой румянец"

func writeShadowedBlush(t *testing.T, pending, confirmed string) string {
	t.Helper()
	mine := validTheme("blush")
	mine.Name = shadowedName
	mine.Contract = currentContract
	path := filepath.Join(t.TempDir(), "themes.json")
	if err := storage.Save(path, stateVersion, state{Themes: []Theme{mine}, Pending: pending, Confirmed: confirmed}); err != nil {
		t.Fatal(err)
	}
	return path
}

// shadowedMine returns the user's former "blush" theme as List serves it and
// fails when any id is served twice.
func shadowedMine(t *testing.T, s *Service) Theme {
	t.Helper()
	seen := map[string]bool{}
	var mine []Theme
	for _, th := range s.List() {
		if seen[th.ID] {
			t.Fatalf("List() serves id %q twice", th.ID)
		}
		seen[th.ID] = true
		if !th.BuiltIn && th.Name == shadowedName {
			mine = append(mine, th)
		}
	}
	if len(mine) != 1 {
		t.Fatalf("List() has %d user themes named %q, want 1", len(mine), shadowedName)
	}
	if mine[0].ID == "blush" || mine[0].Tokens["--bg"] != "#101010" {
		t.Fatalf("user theme served as %+v, want its own tokens under a free id", mine[0])
	}
	return mine[0]
}

func requireActive(t *testing.T, s *Service, id string) {
	t.Helper()
	active, err := s.Active()
	if err != nil {
		t.Fatalf("Active() error = %v", err)
	}
	if active.ID != id || active.BuiltIn {
		t.Fatalf("Active() = %q builtIn %v, want the user theme %q", active.ID, active.BuiltIn, id)
	}
}

// A user theme saved as "blush" before blush was built in stays the user's:
// listed once, selected, deletable, never hidden behind the preset.
func TestStoredThemeWithABuiltInIDGetsAFreeID(t *testing.T) {
	path := writeShadowedBlush(t, "", "blush")
	before, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		t.Fatal(err)
	}

	s := mustServiceAt(t, path)
	after, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("startup rewrote themes.json")
	}
	renamed := shadowedMine(t, s).ID
	requireActive(t, s, renamed)
	preset, err := s.Get("blush")
	if err != nil || !preset.BuiltIn {
		t.Fatalf("Get(blush) = %+v, err=%v, want the built-in preset", preset, err)
	}

	if _, err := s.Save(validTheme("other")); err != nil {
		t.Fatalf("Save(other) error = %v", err)
	}
	s = mustServiceAt(t, path)
	if got := shadowedMine(t, s).ID; got != renamed {
		t.Fatalf("after a write and a restart the user theme is %q, want %q", got, renamed)
	}
	requireActive(t, s, renamed)

	if err := s.Delete(renamed); err != nil {
		t.Fatalf("Delete(%s) error = %v", renamed, err)
	}
	s = mustServiceAt(t, path)
	if _, err := s.Get(renamed); err == nil {
		t.Fatalf("Get(%s) after delete and restart succeeded", renamed)
	}
	active, err := s.Active()
	if err != nil || active.ID != defaultTheme().ID {
		t.Fatalf("Active() after delete = %q, err=%v, want %q", active.ID, err, defaultTheme().ID)
	}
}

func TestPendingOnAShadowedIDFollowsTheRename(t *testing.T) {
	s := mustServiceAt(t, writeShadowedBlush(t, "blush", "blush"))
	requireActive(t, s, shadowedMine(t, s).ID)
}

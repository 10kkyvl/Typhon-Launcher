package install

import (
	"path/filepath"
	"testing"

	"typhon/internal/library"
	"typhon/internal/uierr"
)

func TestExecutableCandidates(t *testing.T) {
	t.Run("empty install dir", func(t *testing.T) {
		s, _, reg := newTestService(t)
		reg.put(library.Game{ID: "g1", Title: "Game"})
		_, err := s.ExecutableCandidates("g1")
		if code := uierr.Code(err); code != "install.empty_destination" {
			t.Fatalf("code = %q (%v), want install.empty_destination", code, err)
		}
	})

	t.Run("unknown game", func(t *testing.T) {
		s, _, _ := newTestService(t)
		if _, err := s.ExecutableCandidates("missing"); err == nil {
			t.Fatal("expected not found")
		}
	})

	t.Run("not started", func(t *testing.T) {
		s := mustServiceAt(t, t.TempDir())
		reg := &fakeRegistrar{}
		reg.put(library.Game{ID: "g1", Title: "Game", InstallDir: t.TempDir()})
		s.library = reg
		if _, err := s.ExecutableCandidates("g1"); err == nil {
			t.Fatal("expected an error before ServiceStartup")
		}
	})

	t.Run("missing directory", func(t *testing.T) {
		s, _, reg := newTestService(t)
		reg.put(library.Game{ID: "g1", Title: "Game", InstallDir: filepath.Join(t.TempDir(), "gone")})
		if _, err := s.ExecutableCandidates("g1"); err == nil {
			t.Fatal("a missing install dir must be an error, not an empty list")
		}
	})

	t.Run("finds game exe", func(t *testing.T) {
		s, _, reg := newTestService(t)
		dir := portableSource(t, t.TempDir(), "Game")
		reg.put(library.Game{ID: "g1", Title: "Game", InstallDir: dir})
		got, err := s.ExecutableCandidates("g1")
		if err != nil {
			t.Fatal(err)
		}
		if len(got) == 0 || got[0].Path != filepath.Join(dir, "Game.exe") {
			t.Fatalf("candidates = %+v, want Game.exe first", got)
		}
	})
}

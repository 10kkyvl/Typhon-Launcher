package library

import (
	"path/filepath"
	"testing"
)

func TestFolderHeldSeesEveryKindOfHold(t *testing.T) {
	cases := []struct {
		name  string
		begin func(s *Service, g Game) (func(), error)
	}{
		{"removal", func(s *Service, g Game) (func(), error) { return s.BeginRemoval(g.ID, g.InstallDir) }},
		{"move", func(s *Service, g Game) (func(), error) { return s.BeginMove(g.ID) }},
		{"update", func(s *Service, g Game) (func(), error) { return s.BeginUpdate(g.ID) }},
		{"card being added", func(s *Service, g Game) (func(), error) { return s.holdForAdding("", "Tool", g.InstallDir) }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := newLibrary(t)
			root := filepath.Join(t.TempDir(), "Games")
			g := addAt(t, s, filepath.Join(root, "Game", "game.exe"), "Game")
			sibling := filepath.Join(root, "Game.partial")
			if s.FolderHeld(g.InstallDir) {
				t.Fatal("held before any hold")
			}
			release, err := c.begin(s, g)
			if err != nil {
				t.Fatal(err)
			}
			for _, dir := range []string{g.InstallDir, filepath.Join(g.InstallDir, "Saves"), root} {
				if !s.FolderHeld(dir) {
					t.Errorf("FolderHeld(%s) = false during a %s", dir, c.name)
				}
			}
			if s.FolderHeld(sibling) {
				t.Errorf("FolderHeld(%s) = true for a folder next to the held one", sibling)
			}
			release()
			if s.FolderHeld(g.InstallDir) {
				t.Fatal("still held after release")
			}
		})
	}
}

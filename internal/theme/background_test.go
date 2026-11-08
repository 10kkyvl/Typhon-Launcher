package theme

import (
	"path/filepath"
	"testing"
)

func TestBackgroundFollowsTheActiveTheme(t *testing.T) {
	s := mustServiceAt(t, filepath.Join(t.TempDir(), "themes.json"))
	if got := s.Background(); got != "#090d17" {
		t.Fatalf("default Background() = %q, want the dark preset's --bg", got)
	}

	if err := s.Apply("light"); err != nil {
		t.Fatalf("Apply(light): %v", err)
	}
	if got := s.Background(); got != "#eef1f4" {
		t.Fatalf("light Background() = %q, want the light preset's --bg", got)
	}

	custom := validTheme("mine")
	custom.Base = BaseLight
	custom.Tokens = map[string]string{"--bg": "#102030"}
	saved, err := s.Save(custom)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := s.Apply(saved.ID); err != nil {
		t.Fatalf("Apply(custom): %v", err)
	}
	if got := s.Background(); got != "#102030" {
		t.Fatalf("custom Background() = %q, want its own --bg", got)
	}

	// A theme that leaves --bg out inherits the colour of its base palette.
	bare := validTheme("bare")
	bare.Base = BaseLight
	bare.Tokens = map[string]string{}
	saved, err = s.Save(bare)
	if err != nil {
		t.Fatalf("Save(bare): %v", err)
	}
	if err := s.Apply(saved.ID); err != nil {
		t.Fatalf("Apply(bare): %v", err)
	}
	if got := s.Background(); got != "#eef1f4" {
		t.Fatalf("bare light Background() = %q, want the light base colour", got)
	}
}

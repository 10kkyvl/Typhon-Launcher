package theme

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"typhon/skills"
)

func skillService(t *testing.T) *Service {
	t.Helper()
	return mustServiceAt(t, filepath.Join(t.TempDir(), "themes.json"))
}

func TestSaveAISkillWritesEveryEmbeddedFile(t *testing.T) {
	dir := t.TempDir()
	got, err := skillService(t).SaveAISkill(dir)
	if err != nil {
		t.Fatalf("SaveAISkill error = %v", err)
	}
	if want := filepath.Join(dir, "typhon-theme"); got != want {
		t.Fatalf("path = %q, want %q", got, want)
	}
	err = fs.WalkDir(skills.FS, skills.ThemeDir, func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil || d.IsDir() {
			return walkErr
		}
		want, err := fs.ReadFile(skills.FS, p)
		if err != nil {
			return err
		}
		have, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(p)))
		if err != nil {
			return err
		}
		if string(have) != string(want) {
			t.Errorf("%s differs from embedded copy", p)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestSaveAISkillRejectsBadDirs(t *testing.T) {
	file := filepath.Join(t.TempDir(), "plain")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	cases := map[string]string{
		"empty":    "",
		"relative": filepath.Join("rel", "dir"),
		"missing":  filepath.Join(t.TempDir(), "nope"),
		"file":     file,
	}
	for name, dir := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := skillService(t).SaveAISkill(dir); !errors.Is(err, ErrSkillPath) {
				t.Fatalf("SaveAISkill(%q) = %v, want ErrSkillPath", dir, err)
			}
		})
	}
}

func TestSaveAISkillRewritesOwnFilesAndKeepsForeign(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "typhon-theme")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	foreign := filepath.Join(root, "mine.txt")
	if err := os.WriteFile(foreign, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	own := filepath.Join(root, "SKILL.md")
	if err := os.WriteFile(own, []byte("stale"), 0o600); err != nil {
		t.Fatal(err)
	}
	s := skillService(t)
	for i := 0; i < 2; i++ {
		if _, err := s.SaveAISkill(dir); err != nil {
			t.Fatalf("SaveAISkill run %d error = %v", i, err)
		}
	}
	want, err := fs.ReadFile(skills.FS, "typhon-theme/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	have, err := os.ReadFile(own)
	if err != nil || string(have) != string(want) {
		t.Fatalf("SKILL.md not refreshed: %v", err)
	}
	if kept, err := os.ReadFile(foreign); err != nil || string(kept) != "keep" {
		t.Fatalf("foreign file changed: %q, %v", kept, err)
	}
}

func TestSaveAISkillFailsWhenTargetIsAFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "typhon-theme"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := skillService(t).SaveAISkill(dir); err == nil {
		t.Fatal("SaveAISkill over a file named typhon-theme returned nil")
	}
}

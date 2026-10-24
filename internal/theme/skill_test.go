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

func TestSaveAISkillLeavesNoStagingFolder(t *testing.T) {
	ok := t.TempDir()
	if _, err := skillService(t).SaveAISkill(ok); err != nil {
		t.Fatal(err)
	}
	blocked := t.TempDir()
	if err := os.WriteFile(filepath.Join(blocked, "typhon-theme"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := skillService(t).SaveAISkill(blocked); !errors.Is(err, ErrSkillWrite) {
		t.Fatalf("SaveAISkill over a file = %v, want ErrSkillWrite", err)
	}
	for _, dir := range []string{ok, blocked} {
		left, err := filepath.Glob(filepath.Join(dir, ".typhon-theme-*"))
		if err != nil || len(left) != 0 {
			t.Errorf("staging folders left in %s: %v, %v", dir, left, err)
		}
	}
	if raw, err := os.ReadFile(filepath.Join(blocked, "typhon-theme")); err != nil || string(raw) != "x" {
		t.Errorf("blocking file changed: %q, %v", raw, err)
	}
}

func TestInstallSkillRollsBackOnFailure(t *testing.T) {
	stage, root := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(stage, "a.md"), []byte("new-a"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "a.md"), []byte("old-a"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := installSkill(stage, root, []string{"a.md", "missing.md"}); err == nil {
		t.Fatal("installSkill succeeded with a file missing from the stage")
	}
	if raw, err := os.ReadFile(filepath.Join(root, "a.md")); err != nil || string(raw) != "old-a" {
		t.Fatalf("a.md after rollback = %q, %v, want old-a", raw, err)
	}
}

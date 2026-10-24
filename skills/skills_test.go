package skills

import (
	"io/fs"
	"strings"
	"testing"
)

func TestThemeSkillHasFrontmatter(t *testing.T) {
	raw, err := fs.ReadFile(FS, ThemeDir+"/SKILL.md")
	if err != nil {
		t.Fatalf("SKILL.md is not embedded: %v", err)
	}
	text := strings.ReplaceAll(string(raw), "\r\n", "\n")
	if !strings.HasPrefix(text, "---\n") {
		t.Fatal("SKILL.md does not start with frontmatter")
	}
	end := strings.Index(text[4:], "\n---")
	if end < 0 {
		t.Fatal("SKILL.md frontmatter is not closed")
	}
	front := text[4 : 4+end]
	if !strings.Contains(front, "name: typhon-theme") {
		t.Errorf("frontmatter lacks name: typhon-theme: %q", front)
	}
	if !strings.Contains(front, "description:") {
		t.Errorf("frontmatter lacks description: %q", front)
	}
}

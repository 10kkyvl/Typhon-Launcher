package theme

import (
	"encoding/json"
	"os"
	"regexp"
	"strings"
	"testing"
)

var skillTokenRow = regexp.MustCompile("(?m)^\\| `(--[a-z0-9-]+)` \\| (\\w+) \\| [^|]*\\| `([^`]*)` \\|$")

func TestSkillTokenReferenceMatchesCode(t *testing.T) {
	data, err := os.ReadFile("../../skills/typhon-theme/tokens.md")
	if err != nil {
		t.Fatalf("read skill reference: %v", err)
	}
	rows := map[string][2]string{}
	for _, m := range skillTokenRow.FindAllStringSubmatch(string(data), -1) {
		rows[m[1]] = [2]string{m[2], m[3]}
	}
	if len(rows) != len(allowedTokens) {
		t.Errorf("skill lists %d tokens, the launcher allows %d", len(rows), len(allowedTokens))
	}
	for _, tok := range allowedTokens {
		row, ok := rows[tok.Name]
		if !ok {
			t.Errorf("%s is missing from the skill reference", tok.Name)
			continue
		}
		if row[0] != string(tok.Kind) {
			t.Errorf("%s: skill says kind %s, code says %s", tok.Name, row[0], tok.Kind)
		}
		if want := presetDark.Tokens[tok.Name]; row[1] != want {
			t.Errorf("%s: skill default %q, dark preset %q", tok.Name, row[1], want)
		}
	}
}

func TestSkillExampleThemeIsAccepted(t *testing.T) {
	data, err := os.ReadFile("../../skills/typhon-theme/SKILL.md")
	if err != nil {
		t.Fatalf("read skill: %v", err)
	}
	text := string(data)
	start := strings.Index(text, "## Example")
	if start < 0 {
		t.Fatal("skill has no Example section")
	}
	block := text[start:]
	open := strings.Index(block, "```json\n")
	if open < 0 {
		open = strings.Index(block, "```json\r\n")
	}
	if open < 0 {
		t.Fatal("Example section has no json block")
	}
	body := block[open+len("```json"):]
	body = body[:strings.Index(body, "```")]
	var file struct {
		Version int   `json:"version"`
		Theme   Theme `json:"theme"`
	}
	dec := json.NewDecoder(strings.NewReader(body))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&file); err != nil {
		t.Fatalf("decode example: %v", err)
	}
	if file.Version != 1 {
		t.Fatalf("example version = %d", file.Version)
	}
	if _, err := Validate(file.Theme); err != nil {
		t.Fatalf("example theme refused: %v", err)
	}
}

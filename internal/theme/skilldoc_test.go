package theme

import (
	"encoding/json"
	"os"
	"regexp"
	"slices"
	"strconv"
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
	for _, m := range skillTokenRow.FindAllStringSubmatch(strings.ReplaceAll(string(data), "\r\n", "\n"), -1) {
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

var skillLayoutRow = regexp.MustCompile("(?m)^\\| `([A-Za-z0-9-]+)` \\|(.*)\\|$")

func skillSections(t *testing.T, path string) map[string]string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	sections := map[string]string{}
	for _, part := range strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n## ")[1:] {
		title, body, _ := strings.Cut(part, "\n")
		sections[title] = body
	}
	return sections
}

func layoutRows(t *testing.T, sections map[string]string, title string) map[string][]string {
	t.Helper()
	body, ok := sections[title]
	if !ok {
		t.Fatalf("layout.md has no section %q", title)
	}
	rows := map[string][]string{}
	for _, m := range skillLayoutRow.FindAllStringSubmatch(body, -1) {
		cells := strings.Split(m[2], "|")
		for i := range cells {
			cells[i] = strings.TrimSpace(cells[i])
		}
		rows[m[1]] = cells
	}
	return rows
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

func checkRows(t *testing.T, title string, rows map[string][]string, want map[string][]string) {
	t.Helper()
	for id, cells := range want {
		got, ok := rows[id]
		if !ok {
			t.Errorf("%s: %q is missing from layout.md", title, id)
			continue
		}
		for i, cell := range cells {
			if i+1 >= len(got) || got[i+1] != cell {
				t.Errorf("%s: %q row = %v, want column %d = %q", title, id, got, i+2, cell)
			}
		}
	}
	for id := range rows {
		if _, ok := want[id]; !ok {
			t.Errorf("%s: layout.md lists %q, which layout.json does not allow", title, id)
		}
	}
}

func TestSkillLayoutReferenceMatchesLayoutJSON(t *testing.T) {
	if layoutSpecErr != nil {
		t.Fatalf("layout.json: %v", layoutSpecErr)
	}
	spec := layoutIDs
	sections := skillSections(t, "../../skills/typhon-theme/layout.md")

	sidebar := map[string][]string{}
	for _, id := range append(slices.Clone(spec.Sidebar.Items), spec.Sidebar.Hideable...) {
		sidebar[id] = []string{yesNo(slices.Contains(spec.Sidebar.Items, id)), yesNo(slices.Contains(spec.Sidebar.Hideable, id))}
	}
	checkRows(t, "Sidebar", layoutRows(t, sections, "Sidebar"), sidebar)

	topbar := map[string][]string{}
	for _, id := range spec.Topbar.Actions {
		topbar[id] = nil
	}
	checkRows(t, "Top bar", layoutRows(t, sections, "Top bar"), topbar)

	game := map[string][]string{}
	for _, id := range spec.GamePage.Main {
		game[id] = []string{"main"}
	}
	for _, id := range spec.GamePage.Side {
		game[id] = []string{"side"}
	}
	checkRows(t, "Game page", layoutRows(t, sections, "Game page"), game)

	parts := map[string][]string{}
	for _, id := range spec.Parts {
		parts[id] = []string{yesNo(slices.Contains(spec.PartsWithID, id))}
	}
	partRows := layoutRows(t, sections, "Parts for custom CSS")
	for id, cells := range partRows {
		partRows[id] = append([]string{""}, cells...)
	}
	checkRows(t, "Parts for custom CSS", partRows, parts)

	plain := map[string][]string{
		"Sidebar": append(slices.Clone(spec.Sidebar.Sides), spec.Separator, strconv.Itoa(spec.MaxSeparators)),
		"Top bar": spec.Topbar.SearchPositions,
	}
	for title, values := range plain {
		for _, v := range values {
			if !strings.Contains(sections[title], "`"+v+"`") && !strings.Contains(sections[title], " "+v+" ") {
				t.Errorf("%s: layout.md does not mention %q", title, v)
			}
		}
	}
	order := make([]string, len(spec.Sidebar.DefaultOrder))
	for i, id := range spec.Sidebar.DefaultOrder {
		order[i] = "`" + id + "`"
	}
	if got := strings.Join(strings.Fields(sections["Sidebar"]), " "); !strings.Contains(got, strings.Join(order, ", ")) {
		t.Errorf("Sidebar: layout.md does not list the default order %s", strings.Join(order, ", "))
	}
}

func TestSkillListsEveryRefusedCSSConstruct(t *testing.T) {
	section, ok := skillSections(t, "../../skills/typhon-theme/SKILL.md")["Custom CSS"]
	if !ok {
		t.Fatal("SKILL.md has no Custom CSS section")
	}
	for _, bad := range append([]string{`\`}, forbiddenCSSSubstrings...) {
		if !strings.Contains(section, "`"+bad+"`") {
			t.Errorf("Custom CSS does not mention %q", bad)
		}
	}
}

func TestSkillExampleThemeIsAccepted(t *testing.T) {
	data, err := os.ReadFile("../../skills/typhon-theme/SKILL.md")
	if err != nil {
		t.Fatalf("read skill: %v", err)
	}
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	start := strings.Index(text, "## Example")
	if start < 0 {
		t.Fatal("skill has no Example section")
	}
	block := text[start:]
	open := strings.Index(block, "```json\n")
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

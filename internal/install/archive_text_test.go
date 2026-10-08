package install

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"unicode"
)

var rightToLeftOverride = string(rune(0x202e))

func assertPlainText(t *testing.T, what, text string) {
	t.Helper()
	for _, r := range text {
		if unicode.IsControl(r) || r == 0x202e || r == unicode.ReplacementChar {
			t.Fatalf("%s %q still carries %U", what, text, r)
		}
	}
}

func TestSanitizeText(t *testing.T) {
	cases := []struct {
		name string
		in   string
		max  int
		want string
	}{
		{name: "plain", in: "Game/data/a.bin", max: 64, want: "Game/data/a.bin"},
		{name: "cyrillic and spaces stay", in: "Игра/файл 1.bin", max: 64, want: "Игра/файл 1.bin"},
		{name: "line break and escape", in: "a\nb\x1b[31m", max: 64, want: "a?b?[31m"},
		{name: "text direction override", in: "exe" + rightToLeftOverride + "txt", max: 64, want: "exe?txt"},
		{name: "invalid utf-8", in: "a\xffb", max: 64, want: "a?b"},
		{name: "truncated", in: strings.Repeat("x", 10), max: 4, want: "xxxx…"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := sanitizeText(tc.in, tc.max); got != tc.want {
				t.Fatalf("sanitizeText(%q, %d) = %q, want %q", tc.in, tc.max, got, tc.want)
			}
		})
	}
}

func TestProgressNamesAreSanitised(t *testing.T) {
	var got []string
	rep := newReporter(func(p Progress) { got = append(got, p.CurrentFile) }, 0)
	rep.setFile("Game/\x1b[2Jevil" + rightToLeftOverride + "txt\nname.bin")
	rep.flush()
	if len(got) == 0 {
		t.Fatal("no progress was reported")
	}
	for _, name := range got {
		assertPlainText(t, "progress file name", name)
	}
}

func TestToolOutputInErrorsIsSanitised(t *testing.T) {
	tail := &tailBuffer{}
	if _, err := tail.Write([]byte("ERROR: \x1b[31mbad" + rightToLeftOverride + " entry\x00 : Game\\a\tb.bin\xff")); err != nil {
		t.Fatal(err)
	}
	assertPlainText(t, "tail text", tail.text())

	useTools(t, fakePlanTool(t, fakePlan{Stderr: "ERROR: CRC Failed : Game\\\x1b[2J" + rightToLeftOverride + "evil.bin\n", Exit: 2}))
	err := ExtractArchive(context.Background(), brokenRar(t), filepath.Join(t.TempDir(), "out"), nil)
	if err == nil || !strings.Contains(err.Error(), "CRC Failed") {
		t.Fatalf("err = %v, want the tool's message", err)
	}
	assertPlainText(t, "error text", err.Error())
}

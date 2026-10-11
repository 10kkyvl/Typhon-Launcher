package redact

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

const slogMsg = "operation failed"

func slogLine(t *testing.T, kv ...any) string {
	t.Helper()
	var buf bytes.Buffer
	h := slog.NewTextHandler(&buf, &slog.HandlerOptions{
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if len(groups) == 0 && a.Key == slog.TimeKey {
				return slog.Attr{}
			}
			return a
		},
	})
	slog.New(h).Error(slogMsg, kv...)
	return strings.TrimSuffix(buf.String(), "\n")
}

type scrubCase struct {
	name  string
	in    string
	leaks []string
	keeps []string
}

func runScrubCases(t *testing.T, cases []scrubCase) {
	t.Helper()
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			outs := map[string]string{"Text": Text(c.in)}
			safe, err := Sanitize(c.in)
			if err != nil {
				t.Fatalf("Sanitize(%q) refused a scrubbable value: %v", c.in, err)
			}
			outs["Sanitize"] = safe
			for fn, got := range outs {
				for _, leak := range c.leaks {
					if strings.Contains(got, leak) {
						t.Fatalf("%s(%q) leaked %q:\n  got %q", fn, c.in, leak, got)
					}
				}
				for _, keep := range c.keeps {
					if !strings.Contains(got, keep) {
						t.Fatalf("%s(%q) dropped %q:\n  got %q", fn, c.in, keep, got)
					}
				}
			}
		})
	}
}

func TestApostropheInsidePath(t *testing.T) {
	runScrubCases(t, []scrubCase{
		{
			name:  "windows",
			in:    `launch C:\Games\Assassin's Creed\a.exe: access denied`,
			leaks: []string{"Creed", "Assassin", "a.exe", `C:\`},
			keeps: []string{"launch", "access denied"},
		},
		{
			name:  "windows forward slashes",
			in:    "open D:/Games/Assassin's Creed/a.exe: not found",
			leaks: []string{"Creed", "Assassin", "a.exe", "D:/"},
			keeps: []string{"open", "not found"},
		},
		{
			name:  "windows escaped by slog",
			in:    slogLine(t, "path", `C:\Games\Assassin's Creed\a.exe`),
			leaks: []string{"Creed", "Assassin", "a.exe", "Games"},
			keeps: []string{slogMsg},
		},
		{
			name:  "unc",
			in:    `copy \\nas\games\Assassin's Creed\a.iso failed`,
			leaks: []string{"Creed", "Assassin", "a.iso", "nas"},
			keeps: []string{"copy"},
		},
		{
			name:  "unix",
			in:    "open /home/egor/Games/Assassin's Creed/save.dat: no such file",
			leaks: []string{"Creed", "Assassin", "save.dat", "egor"},
			keeps: []string{"open", "no such file"},
		},
		{
			name:  "unix without spaces",
			in:    "open /home/egor/Games/Assassin's/save.dat: no such file",
			leaks: []string{"s/save.dat", "Assassin", "egor"},
			keeps: []string{"no such file"},
		},
		{
			name:  "apostrophe in the last component",
			in:    `remove C:\Games\Baldur's: access denied`,
			leaks: []string{"Baldur", "Games"},
			keeps: []string{"remove", "access denied"},
		},
	})
}

func TestQuotedPathStaysQuoted(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"windows", `open 'C:\x\y' failed`, `open '<path>' failed`},
		{"windows at end", `open 'C:\x\y'`, `open '<path>'`},
		{"windows with comma", `paths 'C:\x\y','D:\z'`, `paths '<path>','<path>'`},
		{"unc", `open '\\nas\share\x' failed`, `open '<path>' failed`},
		{"unix", `open '/home/egor/x' failed`, `open '<path>' failed`},
		{"unix at end", `open '/home/egor/x'`, `open '<path>'`},
		{"apostrophe inside quoted windows", `open 'C:\Games\Assassin's Creed\a.exe' failed`, `open '<path>' failed`},
		{"apostrophe inside quoted unix", `open '/home/egor/Assassin's Creed/a.exe' failed`, `open '<path>' failed`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Text(c.in); got != c.want {
				t.Fatalf("Text(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestInstallerOutputInSlogAttribute(t *testing.T) {
	out := "Extracting C:\\Games\\Assassin's Creed\\setup.exe: error 1603\r\n" +
		"C:\\Users\\Egor\\AppData\\Local\\Temp\\is-ABC.tmp\\setup.log: cannot write\n" +
		"D:\\Backup\\Egor Saves\\a.sav: access denied \"quoted\"\n" +
		"\\\\nas\\share\\x.iso: unreachable\n" +
		"/home/egor/prefix/drive_c: no such file\n" +
		"E:\\Last\\Line\\b.bin"
	line := slogLine(t, "component", "install", "output", out)
	if !strings.Contains(line, `\n`) {
		t.Fatalf("fixture lost its literal newline escape: %q", line)
	}
	runScrubCases(t, []scrubCase{{
		name: "installer output",
		in:   line,
		leaks: []string{
			"Egor", "Assassin", "Creed", "setup.exe", "setup.log", "is-ABC", "Backup",
			"a.sav", "nas", "share", "x.iso", "egor", "prefix", "drive_c", "Last", "b.bin",
			"AppData", "C:", "D:", "E:",
		},
		keeps: []string{slogMsg, "component=install", "error 1603", "cannot write", "access denied", "unreachable", "no such file"},
	}})
}

func TestPathAfterLiteralEscapes(t *testing.T) {
	runScrubCases(t, []scrubCase{
		{
			name:  "drive after literal newline",
			in:    `msg="first line\nC:\\Users\\Egor\\x.log"`,
			leaks: []string{"Egor", "x.log", "C:"},
			keeps: []string{"first line"},
		},
		{
			name:  "unix after literal tab",
			in:    `stack="goroutine 1\n\t/home/egor/go/src/x.go:12 +0x1"`,
			leaks: []string{"egor", "x.go"},
			keeps: []string{"goroutine 1", ":12 +0x1"},
		},
	})
}

func TestSeveralPathsInOneLine(t *testing.T) {
	runScrubCases(t, []scrubCase{
		{
			name:  "windows separated by error text",
			in:    `copy C:\a\Secret One\f.txt: denied; then D:\b\Secret Two\g.txt: busy and E:\c\Secret Three\h.txt: gone`,
			leaks: []string{"Secret", "f.txt", "g.txt", "h.txt", "C:", "D:", "E:"},
			keeps: []string{"copy", "denied", "busy", "gone"},
		},
		{
			name:  "windows then unc then unix",
			in:    `a C:\one\x: e1 b \\srv\two\y: e2 c /home/egor/three/z: e3`,
			leaks: []string{"one", "two", "three", "srv", "egor", "x:", "y:"},
			keeps: []string{"e1", "e2", "e3"},
		},
		{
			name:  "location suffix before the next path",
			in:    `at C:\one\a.go:10 +0x1: boom C:\two\b.go:20`,
			leaks: []string{"one", "two", "a.go", "b.go"},
			keeps: []string{"boom", ":10 +0x1", ":20"},
		},
	})
}

func TestEmailIsRedacted(t *testing.T) {
	runScrubCases(t, []scrubCase{
		{
			name:  "alone",
			in:    "alice@example.com",
			leaks: []string{"alice", "example.com"},
			keeps: []string{Email},
		},
		{
			name:  "in an error text",
			in:    "login failed for alice.smith+games@mail.example.co.uk: wrong password",
			leaks: []string{"alice", "smith", "mail.example"},
			keeps: []string{"login failed for " + Email, "wrong password"},
		},
		{
			name:  "quoted",
			in:    `register "alice@example.com" rejected`,
			leaks: []string{"alice", "example.com"},
			keeps: []string{`"` + Email + `"`, "rejected"},
		},
		{
			name:  "single quoted",
			in:    `register 'alice@example.com' rejected`,
			leaks: []string{"alice", "example.com"},
			keeps: []string{"'" + Email + "'"},
		},
		{
			name:  "slog attribute",
			in:    slogLine(t, "email", "alice@example.com"),
			leaks: []string{"alice", "example.com"},
			keeps: []string{slogMsg, Email},
		},
		{
			name:  "two in a row",
			in:    "from a@one.org to b@two.org",
			leaks: []string{"a@", "b@", "one.org", "two.org"},
			keeps: []string{"from " + Email + " to " + Email},
		},
	})
}

func TestEmailRuleLeavesUrlHostsAlone(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"userinfo url", "connect http://alice@feed.example/a/b failed", "connect http://feed.example/<query> failed"},
		{"userinfo with password", "connect http://alice:pw@feed.example:8443/a failed", "connect http://feed.example/<query> failed"},
		{"email as userinfo", "connect https://alice@example.com@feed.example/a failed", "connect https://feed.example/<query> failed"},
		{"bare user at host", "ssh git@github failed", "ssh git@github failed"},
		{"handle without domain", "mention @alice in chat", "mention @alice in chat"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Text(c.in); got != c.want {
				t.Fatalf("Text(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestPathRulesLeaveCleanTextAlone(t *testing.T) {
	for _, in := range []string{
		"",
		"plain failure, nothing sensitive",
		"it's fine, don't panic: and/or retry",
		"can't open the user's profile",
	} {
		t.Run(in, func(t *testing.T) {
			if got := Text(in); got != in {
				t.Fatalf("Text(%q) = %q, want it unchanged", in, got)
			}
			out, err := Sanitize(in)
			if err != nil || out != in {
				t.Fatalf("Sanitize(%q) = %q, %v, want it unchanged", in, out, err)
			}
		})
	}
}

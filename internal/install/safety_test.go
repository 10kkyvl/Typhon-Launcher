package install

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestSafeJoinRejectsUnsafeNames(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "out")
	bad := []string{
		"",
		".",
		"..",
		"../evil.txt",
		`..\evil.txt`,
		"sub/../../evil.txt",
		`sub\..\..\evil.txt`,
		"/evil",
		`\evil`,
		"C:/evil.txt",
		`C:\evil.txt`,
		`\\server\share\evil.txt`,
		"//server/share/evil.txt",
	}
	for _, name := range bad {
		if got, err := safeJoin(dest, name); err == nil {
			t.Fatalf("safeJoin(%q) = %q, want error", name, got)
		}
	}
}

func TestSafeJoinAcceptsLocalNames(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "out")
	good := map[string]string{
		"a.txt":       filepath.Join(dest, "a.txt"),
		"sub/a.txt":   filepath.Join(dest, "sub", "a.txt"),
		`sub\a.txt`:   filepath.Join(dest, "sub", "a.txt"),
		"sub/dir/":    filepath.Join(dest, "sub", "dir"),
		"./sub/a.txt": filepath.Join(dest, "sub", "a.txt"),
	}
	for name, want := range good {
		got, err := safeJoin(dest, name)
		if err != nil {
			t.Fatalf("safeJoin(%q): %v", name, err)
		}
		if got != want {
			t.Fatalf("safeJoin(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestExtractZipBlocksTraversal(t *testing.T) {
	tmp := t.TempDir()
	archive := filepath.Join(tmp, "evil.zip")
	writeZip(t, archive, []zipEntry{
		{name: "../evil1.txt", data: []byte("pwned")},
		{name: `..\evil2.txt`, data: []byte("pwned")},
		{name: `C:\evil3.txt`, data: []byte("pwned")},
		{name: "/evil4.txt", data: []byte("pwned")},
		{name: "sub/../../evil5.txt", data: []byte("pwned")},
		{name: "good/ok.txt", data: []byte("fine")},
	})
	dest := filepath.Join(tmp, "out")

	if err := ExtractArchive(context.Background(), archive, dest, nil); err != nil {
		t.Fatalf("ExtractArchive: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dest, "good", "ok.txt"))
	if err != nil || string(data) != "fine" {
		t.Fatalf("safe entry not extracted: %v %q", err, data)
	}

	entries, err := os.ReadDir(tmp)
	if err != nil {
		t.Fatalf("read tmp: %v", err)
	}
	for _, e := range entries {
		switch e.Name() {
		case "evil.zip", "out":
		default:
			t.Fatalf("unexpected file written outside dest: %s", e.Name())
		}
	}
	for _, name := range []string{"evil1.txt", "evil2.txt", "evil3.txt", "evil4.txt", "evil5.txt"} {
		if exists(filepath.Join(dest, name)) {
			t.Fatalf("malicious entry %s landed in dest", name)
		}
	}
}

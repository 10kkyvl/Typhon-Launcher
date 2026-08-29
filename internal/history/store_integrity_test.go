package history

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewServiceAtRejectsEmptyPath(t *testing.T) {
	if _, err := NewServiceAt(""); err == nil {
		t.Fatal("empty path must not produce a service")
	}
}

func TestCorruptHistoryFailsStartupAndKeepsFile(t *testing.T) {
	cases := []struct {
		name string
		raw  string
	}{
		{"truncated", `[{"id":"a"`},
		{"garbage", `not json at all`},
		{"scalar root", `42`},
		{"unsupported version", `{"version":99,"data":[]}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "history.json")
			if err := os.WriteFile(path, []byte(tc.raw), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := NewServiceAt(path); err == nil {
				t.Fatal("corrupt history must not produce a service")
			}
			got, err := os.ReadFile(filepath.Clean(path))
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tc.raw {
				t.Fatalf("file rewritten: %q", got)
			}
		})
	}
}

func TestMissingHistoryStartsEmptyAndSaves(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.json")
	s, err := NewServiceAt(path)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	if got := s.List(Filter{}); len(got) != 0 {
		t.Fatalf("records = %+v, want none", got)
	}
	if err := s.Record(Record{Kind: KindInstalled, Title: "Game"}); err != nil {
		t.Fatalf("record: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("history not saved: %v", err)
	}
}

package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestReadBaseline(t *testing.T) {
	cases := []struct {
		name    string
		content string
		want    map[string]int
		wantErr bool
	}{
		{name: "empty", content: "", want: map[string]int{}},
		{name: "comments and blanks", content: "# c\n\nwindows 84\n", want: map[string]int{"windows": 84}},
		{name: "tagged key", content: "darwin+devmock 12\nlinux 3\n", want: map[string]int{"darwin+devmock": 12, "linux": 3}},
		{name: "bad shape", content: "windows\n", wantErr: true},
		{name: "bad count", content: "windows x\n", wantErr: true},
		{name: "negative", content: "windows -1\n", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "baseline.txt")
			if err := os.WriteFile(path, []byte(tc.content), 0o600); err != nil {
				t.Fatal(err)
			}
			got, err := readBaseline(path)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("readBaseline() = %v, want error", got)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
			for k, v := range tc.want {
				if got[k] != v {
					t.Fatalf("got[%q] = %d, want %d", k, got[k], v)
				}
			}
		})
	}
}

func TestReadBaselineMissingFileIsEmpty(t *testing.T) {
	got, err := readBaseline(filepath.Join(t.TempDir(), "absent.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("got %v, want empty", got)
	}
}

func TestReadBaselineUnreadable(t *testing.T) {
	dir := t.TempDir()
	if _, err := readBaseline(dir); err == nil {
		t.Fatal("reading a directory as baseline: want error")
	}
}

func TestWriteBaselineRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "baseline.txt")
	in := map[string]int{"windows": 84, "darwin": 105, "darwin+devmock": 107}
	if err := writeBaseline(path, in); err != nil {
		t.Fatal(err)
	}
	out, err := readBaseline(path)
	if err != nil {
		t.Fatal(err)
	}
	for k, v := range in {
		if out[k] != v {
			t.Fatalf("out[%q] = %d, want %d", k, out[k], v)
		}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) == "" || data[0] != '#' {
		t.Fatalf("baseline file should start with a comment, got %q", data)
	}
}

func TestIssuesLine(t *testing.T) {
	cases := map[string]string{
		"0 issues.\n": "0",
		"foo.go:1:1: x (gosec)\n3 issues:\n* gosec: 3\n": "3",
		"no summary here": "",
	}
	for in, want := range cases {
		m := issuesLine.FindStringSubmatch(in)
		got := ""
		if m != nil {
			got = m[1]
		}
		if got != want {
			t.Fatalf("issuesLine(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCountFindingsMissingBinary(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	_, err := countFindings("")
	if err == nil {
		t.Fatal("countFindings without golangci-lint on PATH: want error")
	}
	var exit interface{ ExitCode() int }
	if errors.As(err, &exit) {
		t.Fatalf("missing binary must not be reported as an exit error: %v", err)
	}
}
